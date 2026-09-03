/*
 * Copyright 2026 PANTHEON.tech s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/entguard/entguard/pkg/crypto/aesgcm"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/prompter"
	"github.com/entguard/entguard/service/config"
)

const (
	dbMigrationsTable  = "migrations"
	dbMigrationsColumn = "id"
	dbMigrationName    = "20260603122956-v1.14.0-add-encrypted-fields.sql"

	versionLow  = "v1.13.0" // does not support encryption
	versionHigh = "v1.14.0" // requires encryption

	modeEncrypt   = "encrypt"
	modeReencrypt = "reencrypt"
	modeDecrypt   = "decrypt"
	modeStatus    = "status"
)

type CryptOptions struct {
	// Mode is technically not needed, it can be inferred from presence/absence
	// of OldKey and NewKey. But we anyway require the user to explicitly set
	// Mode, to protect against inferring wrong mode in case the user makes
	// mistakes in providing the keys.
	//
	// After the verification, when we are going to transform the database, we
	// do not use Mode anymore and infer it from the keys.
	Mode string

	OldKeyPath string
	NewKeyPath string

	DBHost         string
	DBPort         uint16
	DBName         string
	DBUser         string
	DBPassword     string
	DBConnPoolSize int
}

func NewCryptCommand() *cobra.Command {
	var opts CryptOptions
	cmd := &cobra.Command{
		Use:   "crypt",
		Short: "Encrypt/reencrypt/decrypt sensitive database fields",
		Example: `
# Make sure that the EntGuard orchestrator is stopped before running these commands, to prevent potential clash and data loss.

# Encrypt previously unencrypted database
egvpn crypt --db-host=hostaddress --mode=` + modeEncrypt + ` --new-key-path=/path/to/keyfile

# Reencrypt already encrypted database (change encryption key, that is, decrypt with old key and immediately encrypt with new key)
egvpn crypt --db-host=hostaddress --mode=` + modeReencrypt + ` --old-key-path=/path/to/oldkeyfile --new-key-path=/path/to/newkeyfile

# Do not modify the database, only tell whether the database is already encrypted
egvpn crypt --db-host=hostaddress --mode=` + modeStatus + `

# Each of the files pointed by --old-key-path and --new-key-path must contain an AES encryption key (random bytes). It can be generated for example by:
openssl rand -out /path/to/keyfile 16

# If the Postgres container with the database is running on the same host where you are using egvpn, you can see the host address required for --db-host using:
docker container inspect eg-postgres | grep "IPAddress"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCrypt(opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.OldKeyPath, "old-key-path", "o", "", "Path to the file containing old key (for decryption)")
	flags.StringVarP(&opts.NewKeyPath, "new-key-path", "n", "", "Path to the file containing new key (for encryption)")
	flags.StringVarP(&opts.Mode, "mode", "m", "",
		fmt.Sprintf("Can be one of '%s', '%s', '%s', '%s' (required)", modeEncrypt, modeReencrypt, modeDecrypt, modeStatus))
	flags.StringVar(&opts.DBHost, "db-host", "172.18.0.2", "Host used for connection to the database.")
	flags.Uint16Var(&opts.DBPort, "db-port", 5432, "Port used for connection to the database (default value should work)")
	flags.StringVar(&opts.DBName, "db-name", "entguard", "Database name used for connection to the database (default value should work)")
	flags.StringVar(&opts.DBUser, "db-user", "postgres", "Username used for connection to the database (default value should work)")
	flags.StringVar(&opts.DBPassword, "db-password", "postgres", "Password used for connection to the database (default value should work)")
	flags.IntVar(&opts.DBConnPoolSize, "db-conn-pool-size", 4, "Pool size used for connection to the database (default value should work)")

	return cmd
}

const decryptWarningPrompt = `
WARNING: You are about to decrypt the database sensitive fields. This may put the sensitive information at risk.

Also, up-to-date version of EntGuard Orchestrator (` + versionHigh + ` or higher) does not work with unencrypted sensitive fields.

Unless you want to migrate the database to an older EntGuard version (` + versionLow + ` or lower), proceeding with decryption is strongly discouraged.

Do you want to continue with the decryption? [y/N]`

func runCrypt(opts CryptOptions) error {
	if opts.Mode == modeDecrypt {
		answer := prompter.Prompt(decryptWarningPrompt)
		if strings.ToLower(answer) != "y" {
			return nil
		}
	}

	err := verifyCryptOptions(opts)
	if err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := connectToDBAndCheckVersion(ctx, opts)
	if err != nil {
		return err
	}

	s, err := detectEncStatus(ctx, conn)
	if err != nil {
		return fmt.Errorf("failed to check encryption status: %v", err)
	}
	if opts.Mode == modeStatus {
		return printEncStatus(s)
	}

	// Check the database state to prevent data loss (for example trying to
	// encrypt plaintext when the plaintext is empty and overwriting actual
	// ciphertext). Now we do early shallow check so we can report potential
	// problem early, later we will do more thorough check.
	if opts.Mode == modeEncrypt && s == encrypted {
		return errors.New("attempted to encrypt already encrypted database")
	} else if opts.Mode == modeReencrypt && s == unencrypted {
		return errors.New("attempted to reencrypt unencrypted database")
	} else if opts.Mode == modeDecrypt && s == unencrypted {
		return errors.New("attempted to decrypt unencrypted database")
	}

	var oldKey, newKey []byte
	if opts.OldKeyPath != "" {
		oldKey, err = aesgcm.ReadKeyFromFile(opts.OldKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read old key: %v", err)
		}
	}
	if opts.NewKeyPath != "" {
		newKey, err = aesgcm.ReadKeyFromFile(opts.NewKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read new key: %v", err)
		}
	}

	fmt.Printf("Beginning %s operation\n", opts.Mode)
	err = transformDatabase(ctx, conn, oldKey, newKey)
	if err != nil {
		return fmt.Errorf("failed to %s database: %v", opts.Mode, err)
	}
	fmt.Println("Finished.")
	return nil
}

func verifyCryptOptions(opts CryptOptions) error {
	switch opts.Mode {
	case modeEncrypt:
		if opts.OldKeyPath != "" || opts.NewKeyPath == "" {
			return fmt.Errorf("mode is set to '%s': old-key-path must NOT be set and new-key-path must be set", modeEncrypt)
		}
	case modeReencrypt:
		if opts.OldKeyPath == "" || opts.NewKeyPath == "" {
			return fmt.Errorf("mode is set to '%s': both old-key-path and new-key-path must be set", modeReencrypt)
		}
	case modeDecrypt:
		if opts.OldKeyPath == "" || opts.NewKeyPath != "" {
			return fmt.Errorf("mode is set to '%s': old-key-path must be set and new-key-path must NOT be set", modeDecrypt)
		}
	case modeStatus:
		if opts.OldKeyPath != "" || opts.NewKeyPath != "" {
			// do not throw error
			fmt.Printf("Note: mode is set to '%s': ignoring old-key-path and new-key-path\n", modeStatus)
		}
	default:
		return fmt.Errorf("unsupported mode: '%s'", opts.Mode)
	}

	return nil
}

func connectToDBAndCheckVersion(ctx context.Context, opts CryptOptions) (*pgxpool.Pool, error) {
	fmt.Println("Connecting to database...")
	conn, err := db.ConnectOnce(ctx, &config.PostgresConfig{
		Host:         opts.DBHost,
		Port:         opts.DBPort,
		Database:     opts.DBName,
		User:         opts.DBUser,
		Password:     opts.DBPassword,
		ConnPoolSize: opts.DBConnPoolSize,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create Postgres connection pool and connect to DB: %w", err)
	}
	fmt.Println("Database connection established.")

	err = conn.QueryRow(
		ctx,
		fmt.Sprintf("SELECT 1 FROM %s WHERE %s=$1", dbMigrationsTable, dbMigrationsColumn),
		dbMigrationName,
	).Scan(nil)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("database schema is outdated and incompatible with encryption. " +
				"Please migrate the database to up-to-date version so that you can use encryption")
		}
		return nil, fmt.Errorf("checking database schema version failed: %v", err)
	}

	return conn, nil
}

type encStatus int

const (
	unencrypted encStatus = iota
	encrypted
	noEncryptableData
	invalidEncStatus
)

func printEncStatus(s encStatus) error {
	switch s {
	case unencrypted:
		fmt.Println("Database is NOT encrypted. It is compatible with EntGuard versions " + versionLow + " and lower" +
			" (assuming correct migrations are applied).")
	case encrypted:
		fmt.Println("Database is encrypted. It is compatible with EntGuard versions " + versionHigh + " and higher" +
			" (assuming correct migrations are applied).")
	case noEncryptableData:
		fmt.Println("Database does not yet contain any data that would need encryption. It is compatible with all EntGuard versions" +
			" (assuming correct migrations are applied).")
	default:
		return errors.New("internal error")
	}

	return nil
}

type secretColumn struct {
	TableName            string
	IDColumnName         string
	PlaintextColumnName  string
	CiphertextColumnName string
}

var (
	// most common columns types: id is int, plaintext is string
	secretColumns = []secretColumn{
		{"devices", "id", "private_key", "private_key_encrypted"},
		{"server_wg_configs", "id", "private_key", "private_key_encrypted"},
		{"totp", "id", "key", "key_encrypted"},
		{"adjacencies", "id", "preshared_key", "preshared_key_encrypted"},
	}
	secretColumnsIdAsString = []secretColumn{
		{"ldaps", "id", "bind_pw", "bind_pw_encrypted"},
	}
	secretColumnsPlaintextAsBytes = []secretColumn{
		{"root_ca", "id", "key", "key_encrypted"},
	}
)

func detectEncStatus(ctx context.Context, conn *pgxpool.Pool) (encStatus, error) {
	allSecretColumns := slices.Concat(secretColumns, secretColumnsIdAsString, secretColumnsPlaintextAsBytes)

	for _, col := range allSecretColumns {
		var ciphertext aesgcm.Ciphertext
		err := conn.QueryRow(
			ctx, fmt.Sprintf("SELECT %s FROM %s LIMIT 1", col.CiphertextColumnName, col.TableName),
		).Scan(
			&ciphertext,
		)

		if err != nil {
			if err == pgx.ErrNoRows {
				continue // so far not any data that would need encryption
			}
			return invalidEncStatus, fmt.Errorf("database query failed: %v", err)
		}

		if ciphertext.IsEmpty() {
			return unencrypted, nil
		} else {
			return encrypted, nil
		}
	}
	return noEncryptableData, nil
}

// transformDatabase encrypts or reencypts or decrypts the database.
// It determines the operation based on whether oldKey and newKey are non-empty.
func transformDatabase(ctx context.Context, conn *pgxpool.Pool, oldKey, newKey []byte) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil && rbErr != pgx.ErrTxClosed {
			err = errors.Join(err, fmt.Errorf("failed to rollback transaction: %w", rbErr))
		}
	}()

	err = transformColumns[int, string](ctx, tx, secretColumns, oldKey, newKey)
	if err != nil {
		return err
	}
	err = transformColumns[string, string](ctx, tx, secretColumnsIdAsString, oldKey, newKey)
	if err != nil {
		return err
	}
	err = transformColumns[int, []byte](ctx, tx, secretColumnsPlaintextAsBytes, oldKey, newKey)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return err
}

type scannedRow[IdT int | string, PlaintextT string | []byte] struct {
	id         IdT
	plaintext  *PlaintextT
	ciphertext aesgcm.Ciphertext
}

// transformColumns encrypts or reencypts or decrypts cols.
// It determines the operation based on whether oldKey and newKey are non-empty.
func transformColumns[IdT int | string, PlaintextT string | []byte](ctx context.Context, tx pgx.Tx, cols []secretColumn, oldKey, newKey []byte) error {
	for _, col := range cols {
		rows, err := tx.Query(ctx, fmt.Sprintf(
			"SELECT %s, %s, %s FROM %s",
			col.IDColumnName, col.PlaintextColumnName, col.CiphertextColumnName, col.TableName,
		))
		if err != nil {
			return fmt.Errorf("database query failed: %v", err)
		}

		var (
			id         IdT
			plaintext  *PlaintextT
			ciphertext aesgcm.Ciphertext
		)
		scannedRows, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (scannedRow[IdT, PlaintextT], error) {
			err := row.Scan(&id, &plaintext, &ciphertext)
			return scannedRow[IdT, PlaintextT]{id, plaintext, ciphertext}, err
		})
		if err != nil {
			return fmt.Errorf("CollectRows failed: %v", err)
		}
		for _, sr := range scannedRows {
			newP, newC, err := computeRecord(oldKey, newKey, sr.plaintext, sr.ciphertext)
			if err != nil {
				return fmt.Errorf("error in computing plaintext or ciphertext for table %s, column %s, rowID %v: %v",
					col.TableName, col.PlaintextColumnName, sr.id, err)
			}

			_, err = tx.Exec(
				ctx, fmt.Sprintf(
					"UPDATE %s SET %s=$1, %s=$2 WHERE %s=$3",
					col.TableName, col.PlaintextColumnName, col.CiphertextColumnName, col.IDColumnName,
				),
				newP, newC, sr.id,
			)
			if err != nil {
				return fmt.Errorf("database query failed: %v", err)
			}
		}
	}
	return nil
}

func computeRecord[T string | []byte](oldKey, newKey []byte, plaintextIn *T, ciphertextIn aesgcm.Ciphertext) (
	plaintextOut T, ciphertextOut aesgcm.Ciphertext, err error,
) {
	// Check the database state to prevent data loss. Formerly we did shallow
	// check, now we do thorough check (for every value) (but we still rely on
	// the fact that plaintext is used if and only if ciphertext is not used).
	//
	// If we detect problem here, it means that some value does not agree with
	// the former shallow check. That means that the database is inconsistent.
	if !ciphertextIn.IsEmpty() && len(oldKey) == 0 {
		return T(""), aesgcm.NewEmptyCiphertext(), errors.New(
			"database seems to be corrupted (expected plaintext record, but found encrypted record). Please restore from backup")
	} else if ciphertextIn.IsEmpty() && len(oldKey) != 0 {
		return T(""), aesgcm.NewEmptyCiphertext(), errors.New(
			"database seems to be corrupted (expected encrypted record not found). Please restore from backup")
	}

	var middleStep T

	if len(oldKey) == 0 {
		if plaintextIn == nil {
			middleStep = T("")
		} else {
			middleStep = *plaintextIn
		}
	} else {
		middleStep, err = aesgcm.Open[T](oldKey, ciphertextIn)
		if err != nil {
			return T(""), aesgcm.NewEmptyCiphertext(), fmt.Errorf("failed to decrypt database record: %v", err)
		}
	}

	if len(newKey) == 0 {
		plaintextOut = middleStep
		ciphertextOut = aesgcm.NewEmptyCiphertext()
	} else {
		plaintextOut = T("")
		ciphertextOut, err = aesgcm.Seal(newKey, middleStep)
		if err != nil {
			return T(""), aesgcm.NewEmptyCiphertext(), fmt.Errorf("failed to encrypt database record: %v", err)
		}
	}

	return plaintextOut, ciphertextOut, nil
}

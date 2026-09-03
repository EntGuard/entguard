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

package aesgcm

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"os"
)

// custom type to make it easier to find it in generated DB queries
type Ciphertext []byte

func NewEmptyCiphertext() Ciphertext {
	return Ciphertext{}
}

func (c Ciphertext) IsEmpty() bool {
	return len(c) == 0
}

func seal(key []byte, plaintext []byte) (Ciphertext, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return NewEmptyCiphertext(), err
	}

	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return NewEmptyCiphertext(), err
	}

	ciphertext := aead.Seal(nil, nil, plaintext, nil)
	return ciphertext, nil
}

func open(key []byte, ciphertext Ciphertext) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aead.Open(nil, nil, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// Seal encrypts and authenticates plaintext
func Seal[T string | []byte](key []byte, plaintext T) (Ciphertext, error) {
	return seal(key, []byte(plaintext))
}

// Open decrypts and authenticates ciphertext
func Open[T string | []byte](key []byte, ciphertext Ciphertext) (T, error) {
	plaintext, err := open(key, ciphertext)
	if err != nil {
		return T([]byte(nil)), err
	}
	return T(plaintext), nil
}

// OpenBytes is the same as Open[[]byte]
func OpenBytes(key []byte, ciphertext Ciphertext) ([]byte, error) {
	return open(key, ciphertext)
}

// OpenString is the same as Open[string]
func OpenString(key []byte, ciphertext Ciphertext) (string, error) {
	plaintext, err := open(key, ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func ReadKeyFromFile(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", path, err)
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("encryption key is empty")
	}
	// check if the provided encryption key is valid; instead of inventing our
	// own checks, try to encrypt dummy value and let crypto library determine
	// if the key is valid
	if _, err := seal(key, []byte("dummy")); err != nil {
		return nil, fmt.Errorf("encryption key is not valid AES key: %v", err)
	}
	return key, nil
}

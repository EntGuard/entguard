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

package rpc

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"

	log "github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/service/user"
)

const (
	AuthHeaderKey = "authorization"
	MfaHeaderKey  = "x-auth-mfa"
)

func (s *VpnCServer) BasicAuth(ctx context.Context) (context.Context, error) {
	auth, err := ExtractHeader(ctx, AuthHeaderKey)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "authorization required")
	}

	name, password, err := GetCredentialsFromHeader(auth)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization")
	}

	ip := clientIP(ctx)
	wait := s.lg.CheckWait(name, ip)
	if wait > 0 {
		s.logger.WithFields(log.Fields{
			"username": name,
			"ip":       ip,
		}).Info("Login attempt temporarily blocked because of too many failed attempts")
		return nil, status.Errorf(codes.ResourceExhausted,
			"too many failed login attempts, please retry in %v seconds", int(math.Ceil(wait.Seconds())))
	}

	u, err := s.um.GetUserByNameAndPassword(ctx, name, password)
	if err != nil {
		if err == user.ErrUserNotFound {
			s.lg.RecordFailedAttempt(name, ip)
			return nil, status.Error(codes.Unauthenticated, `invalid username or password`)
		}
		s.logger.WithError(err).Warn("Failed to authenticate user with password")
		return nil, status.Error(codes.Internal, "authentication failed")
	}

	// With MFA the failures are cleared only after the MFA succeeds, otherwise MFA codes could be guessed endlessly.
	if mfa.AuthenticatorType(u.MFAType) == mfa.NONE {
		s.lg.RecordSuccessfulAttempt(name)
	}

	s.logger.WithFields(log.Fields{
		"userID": u.ID,
	}).Debug("User authenticated with credentials")

	ctx = purgeHeader(ctx, AuthHeaderKey)

	userMD := &UserMetadata{
		UserID:       u.ID,
		Username:     name,
		MFAType:      u.MFAType,
		Notification: u.Notification,
	}
	newCtx := SetUserMetadata(ctx, userMD)

	return newCtx, nil
}

func (s *VpnCServer) MfaAuth(ctx context.Context) (context.Context, error) {
	md, ok := GetUserMetadata(ctx)
	if !ok {
		return nil, errors.New("failed to acquire user MD")
	}

	if mfa.AuthenticatorType(md.MFAType) != mfa.NONE {
		var errInfo = &errdetails.ErrorInfo{
			Metadata: map[string]string{
				"mfaType": md.MFAType,
			},
		}

		s.logger.WithField("mfaType", md.MFAType).Debug("MFA is required for user")

		mfaAuth, err := ExtractHeader(ctx, MfaHeaderKey)
		if err != nil {
			s.logger.WithError(err).Debug("Failed to extract MFA header")
			st := status.Newf(codes.Unauthenticated, "MFA required")
			errInfo.Reason = err.Error()
			st = withDetails(st, errInfo)
			return nil, st.Err()
		}
		mfaTyp, mfaData, err := getMfaFromHeader(mfaAuth)
		if err != nil {
			s.logger.WithError(err).Debug("Failed to get MFA data from header")
			st := status.Newf(codes.Unauthenticated, "MFA invalid")
			errInfo.Reason = err.Error()
			st = withDetails(st, errInfo)
			return nil, st.Err()
		}

		// calling CheckWait here in MfaAuth would be redundant, because it has
		// been already called inside BasicAuth

		var authenticated bool

		switch mfa.AuthenticatorType(mfaTyp) {
		case mfa.TOTP:
			s.logger.Debugf("Trying to authenticate user with %v", mfa.TOTP)

			authenticated, err = s.um.MFAAuthenticate(ctx, md.UserID, mfa.TOTP, &mfa.TOTPCredentials{
				Password: mfaData,
			})
		case mfa.CERT:
			s.logger.Debugf("Trying to authenticate user with %v", mfa.CERT)

			authenticated, err = s.um.MFAAuthenticate(ctx, md.UserID, mfa.CERT, &mfa.CertCredential{
				Cert: []byte(mfaData),
			})
		default:
			s.logger.Debugf("Invalid MFA type: %q", mfaTyp)

			st := status.Newf(codes.Unauthenticated, "MFA invalid")
			errInfo.Reason = fmt.Sprintf("unknown MFA type: %q", mfaTyp)
			st = withDetails(st, errInfo)
			return nil, st.Err()
		}
		if err != nil {
			s.logger.WithError(err).Debug("Failed to authenticate user due to an error")
		}
		if err != nil || !authenticated {
			s.lg.RecordFailedAttempt(md.Username, clientIP(ctx))
			s.logger.Debug("Failed to authenticate MFA")
			st := status.Newf(codes.Unauthenticated, "MFA failed")
			if err != nil {
				errInfo.Reason = err.Error()
			} else {
				errInfo.Reason = "MFA authentication failed"
			}
			st = withDetails(st, errInfo)
			return nil, st.Err()
		}

		s.lg.RecordSuccessfulAttempt(md.Username)

		s.logger.WithFields(log.Fields{
			"userID": md.UserID,
		}).Debug("User authenticated with MFA")
	}

	return purgeHeader(ctx, MfaHeaderKey), nil
}

// clientIP returns the address of the connected peer (without port), or empty string if it is unknown.
func clientIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return p.Addr.String()
	}
	return host
}

func withDetails(status *status.Status, errInfo *errdetails.ErrorInfo) *status.Status {
	st, err := status.WithDetails(errInfo)
	if err != nil {
		panic(err) // stop the program execution immediately based on security aspects
	}
	return st
}

func getMfaFromHeader(mfaHeader string) (typ, data string, err error) {
	parts := strings.SplitN(mfaHeader, " ", 2)

	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid MFA header format")
	}

	c, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf(`invalid MFA data in header`)
	}

	typ = parts[0]
	data = string(c)

	if len(typ) == 0 || len(data) == 0 {
		return "", "", fmt.Errorf(`missing MFA value in header`)
	}

	return typ, data, nil
}

func GetCredentialsFromHeader(authHeader string) (name, password string, err error) {
	const prefix = "Basic "

	if !strings.HasPrefix(authHeader, prefix) {
		return "", "", fmt.Errorf(`missing prefix %q in "Authorization" header`, prefix)
	}

	c, err := base64.StdEncoding.DecodeString(authHeader[len(prefix):])
	if err != nil {
		return "", "", fmt.Errorf(`invalid base64 in header`)
	}

	cs := string(c)
	idx := strings.IndexByte(cs, ':')
	if idx < 0 {
		return "", "", fmt.Errorf(`invalid basic auth format`)
	}

	name, password = cs[:idx], cs[idx+1:]
	if len(name) == 0 || len(password) == 0 {
		return "", "", fmt.Errorf(`name or password are missing in the header`)
	}

	return name, password, nil
}

func ExtractHeader(ctx context.Context, header string) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("missing metadata in request")
	}

	authHeaders, ok := md[header]
	if !ok {
		return "", fmt.Errorf("missing header in request")
	}
	if len(authHeaders) != 1 {
		return "", fmt.Errorf("multiple header values in request")
	}

	return authHeaders[0], nil
}

func purgeHeader(ctx context.Context, header string) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)
	mdCopy := md.Copy()
	mdCopy[header] = nil
	return metadata.NewIncomingContext(ctx, mdCopy)
}

type userMDKey struct{}

// UserMetadata contains metadata about a user.
type UserMetadata struct {
	UserID       int
	Username     string
	MFAType      string
	Notification string
}

// GetUserMetadata can be used to extract user metadata stored in a context.
func GetUserMetadata(ctx context.Context) (*UserMetadata, bool) {
	userMD := ctx.Value(userMDKey{})

	switch md := userMD.(type) {
	case *UserMetadata:
		return md, true
	default:
		return nil, false
	}
}

func SetUserMetadata(ctx context.Context, userMD *UserMetadata) context.Context {
	return context.WithValue(ctx, userMDKey{}, userMD)
}

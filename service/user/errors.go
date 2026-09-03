/*
 * Copyright 2021 PANTHEON.tech s.r.o.
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

package user

import "errors"

// List of exportable errors.
var (
	ErrNotFound      = errors.New("not found")
	ErrInvalidData   = errors.New("validation error")
	ErrAlreadyExists = errors.New("already exists")
	ErrQueryFailed   = errors.New("failed to query a database (check logs)")

	ErrUserNotFound    = errors.New("user not found")
	ErrMFACertNotFound = errors.New("mfa certificate entry not found")

	ErrBadPassword = errors.New("the provided password does not fullfill the requirements")
	ErrBadUserData = errors.New("provided user data is invalid")

	ErrFeaturesRestriction = errors.New("feature not allowed, action restricted")

	ErrOnlyForIntrUsers = errors.New("action failed because this user info was imported from another service")

	ErrSyncNotStarted = errors.New("sync process has not started yet")
	ErrSyncInProgress = errors.New("sync stats are in progress")

	ErrAuthenticatorIsNotSupported = errors.New("this type of authenticator is not supported or enabled")
	ErrNoAuthenticatorForUser      = errors.New("user does not have any 2FA authentication set")
	ErrUserSecretNotFound          = errors.New("user does not have any secret for their authentication type")

	ErrDPULimitExceeded         = errors.New("DPU limit exceeded")
	ErrNoAvailableAddressInPool = errors.New("no available address in pool")
)

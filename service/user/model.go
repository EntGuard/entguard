/*
 * Copyright 2020 PANTHEON.tech s.r.o.
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

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/service/api/v1"
)

const (
	// maxUserNameLen defines length limit for user name.
	maxUserNameLen = 50

	// maxNotificationLen defines length limit for user notification message.
	maxNotificationLen = 75

	// List of allowed auth services:
	InternalUser = "intr"
	LDAPUser     = "ldap"
)

type UserModel struct {
	ID           int
	Name         string
	PasswordHash string
	IsAdmin      bool
	AuthService  string
	MfaAuthType  string
	Notification string
	UpdatedAt    time.Time
}

// List of validation errors.
var (
	ErrTooLongUsername     = fmt.Errorf("user name length cannot exceed %d characters", maxUserNameLen)
	ErrTooLongNotification = fmt.Errorf("user notification length cannot exceed %d characters", maxNotificationLen)
	ErrTooSmallUserID      = errors.New("user id is invalid")
)

func NewUserModelWithName(name string) (*UserModel, error) {
	u := &UserModel{}
	err := u.SetAuthType(InternalUser)
	if err != nil {
		return nil, err
	}
	u.SetMFAType(mfa.NONE)
	err = u.SetName(name)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func NewUserModelWithID(id int) (*UserModel, error) {
	u := &UserModel{}
	err := u.SetAuthType(InternalUser)
	if err != nil {
		return nil, err
	}
	u.SetMFAType(mfa.NONE)
	err = u.SetID(id)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (u *UserModel) IsInternal() bool {
	return u.AuthService == InternalUser
}

func (u *UserModel) IsNotInternal() bool {
	return u.AuthService != InternalUser
}

func (u *UserModel) IsLDAP() bool {
	return u.AuthService == LDAPUser
}

func (u *UserModel) MarkAsLDAP() {
	u.AuthService = LDAPUser
}

func (u *UserModel) SetID(id int) error {
	if id < 1 {
		return ErrTooSmallUserID
	}
	u.ID = id
	return nil
}

func (u *UserModel) SetAuthType(t string) error {
	switch t {
	case InternalUser:
		u.AuthService = InternalUser
	case LDAPUser:
		u.AuthService = LDAPUser
	default:
		return fmt.Errorf("unknown user authentication type=%s", t)
	}
	return nil
}

func (u *UserModel) SetName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: username cannot be an empty string", ErrBadUserData)
	}
	if len(name) > maxUserNameLen {
		return fmt.Errorf("%w: %v", ErrBadUserData, ErrTooLongUsername)
	}
	u.Name = name
	return nil
}

func (u *UserModel) SetPassword(pw string) error {
	// hashingCost should be set to a value that takes at least 250 milliseconds on modern computer
	return u.SetPasswordWithHashingCost(pw, 13)
}

func (u *UserModel) SetPasswordWithHashingCost(pw string, hashingCost int) error {
	if pw == "" {
		return fmt.Errorf("%w: password cannot be an empty string", ErrBadUserData)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pw), hashingCost)
	if err != nil {
		return fmt.Errorf("could not hash password: %v", err)
	}
	u.PasswordHash = string(hash)
	return nil
}

func (u *UserModel) SetAdmin(isAdmin bool) {
	u.IsAdmin = isAdmin
}

func (u *UserModel) SetMFAType(t string) {
	if len(t) == 0 {
		u.MfaAuthType = string(mfa.NONE)
		return
	}
	u.MfaAuthType = t
}

func (u *UserModel) SetNotification(t string) error {
	if len(t) > maxNotificationLen {
		return ErrTooLongNotification
	}
	u.Notification = t
	return nil
}

// ComparePasswords returns true if passwords match
func (u *UserModel) ComparePasswords(pw string) bool {
	if pw == "" || u.PasswordHash == "" {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(pw))
	return err == nil
}

func (u *UserModel) ToAPI() api.User {
	return api.User{
		ID:           u.ID,
		Username:     u.Name,
		IsAdmin:      u.IsAdmin,
		MFAType:      u.MfaAuthType,
		AuthType:     u.AuthService,
		Notification: u.Notification,
		UpdatedAt:    u.UpdatedAt,
	}
}

func ConvertDBToUserModel(u sqlc.GetAllUsersRow) *UserModel {
	return &UserModel{
		ID:           u.ID,
		Name:         u.Username,
		IsAdmin:      u.IsAdmin,
		MfaAuthType:  string(u.MfaAuth),
		AuthService:  string(u.AuthService),
		Notification: u.Ntf,
		UpdatedAt:    u.UpdatedAt.Time,
	}
}

func ConvertDBToAPIUser(u sqlc.GetAllUsersRow) api.User {
	return api.User{
		ID:           u.ID,
		Username:     u.Username,
		IsAdmin:      u.IsAdmin,
		MFAType:      string(u.MfaAuth),
		AuthType:     string(u.AuthService),
		Notification: u.Ntf,
		UpdatedAt:    u.UpdatedAt.Time,
	}
}

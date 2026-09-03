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

package config

import (
	"fmt"
	"strconv"
	"time"
)

// FeaturesVersion defines the features version
type FeaturesVersion int

func (v FeaturesVersion) String() string { return strconv.Itoa(int(v)) }

// These constants define all features versions
const (
	// Version1 is deprecated and no longer supported

	Version2 FeaturesVersion = 2
)

// FeaturesType defines features type.
type FeaturesType string

// These constants define possible types for features.
const (
	// community FeaturesType is used when running open-source version of orchestrator (premium features are disabled)
	community FeaturesType = "Community"
)

// FeaturesData defines methods that will be used by Orchestrator to get information related to the features.
type FeaturesData interface {
	Version() FeaturesVersion
	Expires() time.Time
	Starts() time.Time
	LDAP() bool
	Telemetry() bool
	GetMaxUserQuantity() int
	GetDPULimit() int
	Details() string
	String() string
}

// RetrieveFeatures always returns community stub features for open-source version.
func RetrieveFeatures(base64Features, base64PublicKey string) (FeaturesData, error) {
	return &featuresDataV2{
		Type:           community,
		MaxUsers:       0,
		MaxDPU:         1,
		StartDate:      time.Now(),
		ExpirationDate: time.Now().AddDate(1000, 0, 0),
	}, nil
}

// featuresDataV2 holds all the information related to valid version 2 features.
type featuresDataV2 struct {
	Type           FeaturesType `json:"type"`
	MaxUsers       int          `json:"maxusers"`
	MaxDPU         int          `json:"maxdpu"`
	StartDate      time.Time    `json:"startdate"`
	ExpirationDate time.Time    `json:"expirationdate"`

	// Features additional information.
	FeaturesDetails string `json:"details"`
}

// GetMaxUserQuantity returns max user quantity you could create in orchestrator.
// If the return value is zero then the quantity is unlimited.
func (f *featuresDataV2) GetMaxUserQuantity() int {
	return 0
}

func (f *featuresDataV2) GetDPULimit() int {
	return 1
}

func (f *featuresDataV2) LDAP() bool {
	return false
}

func (f *featuresDataV2) Telemetry() bool {
	return false
}

func (f *featuresDataV2) Version() FeaturesVersion {
	return Version2
}

func (f *featuresDataV2) Expires() time.Time {
	return f.ExpirationDate
}

func (f *featuresDataV2) Starts() time.Time {
	return f.StartDate
}

func (f *featuresDataV2) Details() string {
	if len(f.FeaturesDetails) == 0 {
		return "no details specified"
	}
	return f.FeaturesDetails
}

func (f *featuresDataV2) String() string {
	return fmt.Sprintf("Features data:\n"+
		"\tFeatures version: %v\n"+
		"\tFeatures type: %v\n"+
		"\tMaximum number of users: %v\n"+
		"\tMaximum number of devices per user: %v\n"+
		"\tStart date: %v\n"+
		"\tExpiration date: %v\n"+
		"\tFeatures Information: %q",

		Version2,
		f.Type,
		f.MaxUsers,
		f.MaxDPU,
		f.StartDate.Format("2006-01-02T15:04:05Z07:00"),
		f.ExpirationDate.Format("2006-01-02T15:04:05Z07:00"),
		f.FeaturesDetails,
	)
}

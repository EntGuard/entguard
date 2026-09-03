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

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// CreateTempFileWithContent creates a temporary file for given test and with given content. It returns its filepath.
// The file is automatically cleaned after test end.
func CreateTempFileWithContent(t *testing.T, fileName string, content []byte) string {
	tmpDir := t.TempDir() // different for each test and automatically cleaned up
	tmpFilePath := filepath.Join(tmpDir, fileName)
	err := os.WriteFile(tmpFilePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	return tmpFilePath
}

// CreateFileWithContentInExistingDir creates a file with given content in given existing directory. It returns its
// filepath or error. It fails if the directory doesn't exist. There is NO automatic file clean up at test end.
func CreateFileWithContentInExistingDir(existingDir string, fileName string, content []byte) (string, error) {
	filePath := filepath.Join(existingDir, fileName)
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %v", err)
	}
	return filePath, nil
}

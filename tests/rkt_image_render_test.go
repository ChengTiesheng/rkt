// Copyright 2015 The rkt Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coreos/rkt/Godeps/_workspace/src/github.com/steveeJ/gexpect"
)

const (
	manifestRenderTemplate = `{"acKind":"ImageManifest","acVersion":"0.6.1","name":"IMG_NAME","labels":[{"name":"version","value":"1.0.0"},{"name":"arch","value":"amd64"},{"name":"os","value":"linux"}],"dependencies":[{"imageName":"coreos.com/rkt-inspect"}],"app":{"exec":["/inspect"],"user":"0","group":"0","workingDirectory":"/","environment":[{"name":"VAR_FROM_MANIFEST","value":"manifest"}]}}`
)

// TestImageRender tests 'rkt image render', it will import some existing empty
// image with a dependency on an image with the inspect binary, render it with
// rkt image render and check that the exported image has the /inspect file and
// that its hash matches the original inspect binary hash
func TestImageRender(t *testing.T) {
	baseImage := os.Getenv("RKT_INSPECT_IMAGE")
	if baseImage == "" {
		panic("Empty RKT_INSPECT_IMAGE environment variable")
	}
	emptyImage := os.Getenv("RKT_EMPTY_IMAGE")
	if emptyImage == "" {
		panic("Empty RKT_INSPECT_IMAGE environment variable")
	}
	testImageName := "coreos.com/rkt-image-render-test"
	inspectFile := os.Getenv("INSPECT_BINARY")
	if inspectFile == "" {
		panic("Empty INSPECT_BINARY environment variable")
	}
	inspectHash, err := getHash(inspectFile)
	if err != nil {
		panic("Cannot get inspect binary's hash")
	}

	expectManifest := strings.Replace(manifestRenderTemplate, "IMG_NAME", testImageName, -1)

	tmpDir, err := ioutil.TempDir("", "rkt-TestImageRender-")
	if err != nil {
		panic(fmt.Sprintf("Cannot create temp dir: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	tmpManifest, err := ioutil.TempFile(tmpDir, "manifest")
	if err != nil {
		panic(fmt.Sprintf("Cannot create temp manifest: %v", err))
	}
	if err := ioutil.WriteFile(tmpManifest.Name(), []byte(expectManifest), 0600); err != nil {
		panic(fmt.Sprintf("Cannot write to temp manifest: %v", err))
	}
	defer os.Remove(tmpManifest.Name())

	testImage := patchACI(emptyImage, "rkt-inspect-image-render.aci", "--manifest", tmpManifest.Name())
	defer os.Remove(testImage)
	ctx := newRktRunCtx()
	defer ctx.cleanup()

	_ = importImageAndFetchHash(t, ctx, baseImage)
	testImageShortHash := importImageAndFetchHash(t, ctx, testImage)

	tests := []struct {
		image        string
		shouldFind   bool
		expectedHash string
	}{
		{
			testImageName,
			true,
			inspectHash,
		},
		{
			testImageShortHash,
			true,
			inspectHash,
		},
		{
			"sha512-not-existed",
			false,
			"",
		},
		{
			"some~random~aci~name",
			false,
			"",
		},
	}

	for i, tt := range tests {
		outputPath := filepath.Join(tmpDir, fmt.Sprintf("rendered-%d", i))
		runCmd := fmt.Sprintf("%s image render --rootfs-only %s %s", ctx.cmd(), tt.image, outputPath)
		t.Logf("Running 'image render' test #%v: %v", i, runCmd)
		child, err := gexpect.Spawn(runCmd)
		if err != nil {
			t.Fatalf("Cannot exec rkt #%v: %v", i, err)
		}

		if err := child.Wait(); err != nil {
			if !tt.shouldFind && err.Error() == "exit status 1" {
				continue
			} else if tt.shouldFind || err.Error() != "exit status 1" {
				t.Fatalf("rkt didn't terminate correctly: %v", err)
			}
		}

		renderedInspectHash, err := getHash(filepath.Join(outputPath, "inspect"))
		if err != nil {
			t.Fatalf("Cannot get rendered inspect binary's hash")
		}
		if renderedInspectHash != tt.expectedHash {
			t.Fatalf("Expected /inspect hash %q but got %s", tt.expectedHash, renderedInspectHash)
		}
	}
}

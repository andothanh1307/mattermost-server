// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package commands

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-server/api4"
	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/testlib"
)

var coverprofileCounters map[string]int = make(map[string]int)

var mainHelper *testlib.MainHelper

type testHelper struct {
	*api4.TestHelper

	config         *model.Config
	tempDir        string
	configFilePath string
}

func Setup() *testHelper {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		panic("failed to create temporary directory: " + err.Error())
	}

	api4TestHelper := api4.Setup()

	testHelper := &testHelper{
		TestHelper:     api4TestHelper,
		tempDir:        dir,
		configFilePath: filepath.Join(dir, "config.json"),
	}

	config := &model.Config{}
	config.SetDefaults()
	testHelper.SetConfig(config)

	return testHelper
}

func (h *testHelper) InitBasic() *testHelper {
	h.TestHelper.InitBasic()
	return h
}

func (h *testHelper) Config() *model.Config {
	return h.config.Clone()
}

func (h *testHelper) ConfigPath() string {
	return h.configFilePath
}

func (h *testHelper) SetConfig(config *model.Config) {
	config.SqlSettings = *mainHelper.Settings
	h.config = config

	if err := ioutil.WriteFile(h.configFilePath, []byte(config.ToJson()), 0600); err != nil {
		panic("failed to write file " + h.configFilePath + ": " + err.Error())
	}
}

func (h *testHelper) TearDown() {
	h.TestHelper.TearDown()
	os.RemoveAll(h.tempDir)
}

func (h *testHelper) execArgs(t *testing.T, args []string) []string {
	ret := []string{"-test.v", "-test.run", "ExecCommand"}
	if coverprofile := flag.Lookup("test.coverprofile").Value.String(); coverprofile != "" {
		dir := filepath.Dir(coverprofile)
		base := filepath.Base(coverprofile)
		baseParts := strings.SplitN(base, ".", 2)
		coverprofileCounters[t.Name()] = coverprofileCounters[t.Name()] + 1
		baseParts[0] = fmt.Sprintf("%v-%v-%v", baseParts[0], t.Name(), coverprofileCounters[t.Name()])
		ret = append(ret, "-test.coverprofile", filepath.Join(dir, strings.Join(baseParts, ".")))
	}

	ret = append(ret, "--", "--disableconfigwatch")

	// Unless the test passes a `--config` of its own, create a temporary one from the default
	// configuration with the current test database applied.
	hasConfig := false
	for _, arg := range args {
		if arg == "--config" {
			hasConfig = true
			break
		}
	}

	if !hasConfig {
		ret = append(ret, "--config", h.configFilePath)
	}

	ret = append(ret, args...)

	return ret
}

// CheckCommand invokes the test binary, returning the output modified for assertion testing.
func (h *testHelper) CheckCommand(t *testing.T, args ...string) string {
	path, err := os.Executable()
	require.NoError(t, err)
	output, err := exec.Command(path, h.execArgs(t, args)...).CombinedOutput()
	require.NoError(t, err, string(output))
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(string(output)), "PASS"))
}

// RunCommand invokes the test binary, returning only any error.
func (h *testHelper) RunCommand(t *testing.T, args ...string) error {
	path, err := os.Executable()
	require.NoError(t, err)
	return exec.Command(path, h.execArgs(t, args)...).Run()
}

// RunCommandWithOutput is a variant of RunCommand that returns the unmodified output and any error.
func (h *testHelper) RunCommandWithOutput(t *testing.T, args ...string) (string, error) {
	path, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(path, h.execArgs(t, args)...)

	var buf bytes.Buffer
	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer

	done := make(chan bool)
	go func() {
		io.Copy(&buf, reader)
		close(done)
	}()

	err = cmd.Run()
	writer.Close()
	<-done

	return buf.String(), err
}

// internal/cli/chat_test.go
package agon

import (
	"bytes"
	"os"
	"sync"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
)

// TestChatCmd tests the functionality of the chat command. It ensures that the
// command correctly loads the application configuration and invokes the
// graphical user interface (GUI) with the appropriate settings. This test
// simulates a scenario where a temporary configuration file is created, and the
// chat command is executed, verifying that the GUI is started with the correct
// configuration.
func TestChatCmd(t *testing.T) {
	tempFile, err := os.CreateTemp("", "config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	configJSON := `{
        "hosts": [
            {
                "name": "Test Host",
                "url": "http://localhost:11434",
                "type": "ollama",
                "models": ["model1"]
            }
        ]
    }`
	if _, err := tempFile.WriteString(configJSON); err != nil {
		t.Fatal(err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatal(err)
	}

	loadedCfg, err := appconfig.Load(tempFile.Name())
	if err != nil {
		t.Fatalf("failed to load temp config: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetErr(b)

	originalStartGUI := startGUI
	defer func() { startGUI = originalStartGUI }()
	defer func() {
		cfgOnce = sync.Once{}
		cfgLoadErr = nil
		currentConfig = nil
	}()

	cfgOnce = sync.Once{}
	cfgLoadErr = nil
	currentConfig = &loadedCfg

	startCalled := false
	var receivedCfg *appconfig.Config
	startGUI = func(cfg *appconfig.Config) {
		startCalled = true
		receivedCfg = cfg
	}

	chatCmd.Run(chatCmd, []string{})

	if !startCalled {
		t.Fatal("expected startGUI to be invoked")
	}
	if receivedCfg == nil {
		t.Fatal("expected to receive a config instance")
	}
	if receivedCfg != getConfig() {
		t.Fatal("expected startGUI to receive the loaded configuration")
	}
}
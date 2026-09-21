package notehistory

import (
	"bytes"
	"testing"

	"howett.net/plist"
)

func TestAgentPlistRendersEnvironment(t *testing.T) {
	var buf bytes.Buffer
	err := agentPlist.Execute(&buf, map[string]any{
		"Label": agentLabel, "Binary": "/x/notehistory", "Log": "/x/log",
		"Env": map[string]string{"NOTIFICATION_KEEP_DAYS": "30"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var agent struct {
		Args []string          `plist:"ProgramArguments"`
		Env  map[string]string `plist:"EnvironmentVariables"`
	}
	if _, err := plist.Unmarshal(buf.Bytes(), &agent); err != nil {
		t.Fatalf("rendered plist does not parse: %v\n%s", err, buf.String())
	}
	if agent.Env["NOTIFICATION_KEEP_DAYS"] != "30" || len(agent.Args) != 2 {
		t.Fatalf("got %+v", agent)
	}

	t.Run("no environment block when nothing set", func(t *testing.T) {
		buf.Reset()
		agentPlist.Execute(&buf, map[string]any{"Label": agentLabel, "Binary": "/x", "Log": "/x", "Env": map[string]string{}})
		if bytes.Contains(buf.Bytes(), []byte("EnvironmentVariables")) {
			t.Error("empty env rendered an EnvironmentVariables block")
		}
	})
}

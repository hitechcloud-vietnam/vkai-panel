package phpfpm

import (
	"strings"
	"testing"
)

// validSpec is a pool that renders. Each test below breaks exactly one field,
// so a failure names the field that stopped being validated.
func validSpec() *PoolSpec {
	return &PoolSpec{
		Name:              "example-com",
		Version:           "8.3",
		User:              "site-example",
		Group:             "site-example",
		ListenOwner:       "www-data",
		ListenGroup:       "www-data",
		ListenMode:        "0660",
		PM:                "dynamic",
		PMMaxChildren:     20,
		PMStartServers:    3,
		PMMinSpareServers: 2,
		PMMaxSpareServers: 6,
		PMMaxRequests:     500,
		MemoryLimit:       "512M",
		MaxExecutionTime:  120,
		MaxInputTime:      60,
		UploadMaxFilesize: "128M",
		PostMaxSize:       "128M",
		MaxFileUploads:    20,
		Timezone:          "Asia/Ho_Chi_Minh",
		Extensions:        []string{"redis", "imagick"},
		DisabledFunctions: []string{"system", "exec"},
		OpenBasedir:       []string{"/vkai-panel/www/domains/example.com", "/tmp"},
		ErrorLog:          "/vkai-panel/logs/sites/example.com/php-error.log",
		SocketPath:        "/run/php/example-com.sock",
	}
}

// TestPoolFileCarriesEveryPerSiteSetting is the assertion the task's second
// requirement asks for: memory limit, max execution time, upload size and the
// extension set must actually reach the pool file, not merely be stored.
func TestPoolFileCarriesEveryPerSiteSetting(t *testing.T) {
	rendered, err := validSpec().Render()
	if err != nil {
		t.Fatalf("a valid pool spec did not render: %v", err)
	}
	text := string(rendered)

	// php_admin_value, not php_value: a site must not be able to raise its own
	// limits back with ini_set(). This is the assertion that catches somebody
	// "simplifying" the renderer.
	required := []struct {
		line string
		why  string
	}{
		{"[example-com]", "the pool section header names the pool"},
		{"user = site-example", "the pool runs as the site's own user, which is the isolation boundary"},
		{"group = site-example", "the pool's group"},
		{"listen = /run/php/example-com.sock", "the web server has nothing to talk to without this"},
		{"listen.mode = 0660", "the socket permissions"},
		{"pm = dynamic", "the process manager"},
		{"pm.max_children = 20", "the process manager's ceiling"},
		{"php_admin_value[memory_limit] = 512M", "memory limit must reach the pool file"},
		{"php_admin_value[max_execution_time] = 120", "max execution time must reach the pool file"},
		{"php_admin_value[upload_max_filesize] = 128M", "upload size must reach the pool file"},
		{"php_admin_value[post_max_size] = 128M", "an upload limit without a matching post size does nothing"},
		{"php_admin_value[date.timezone] = Asia/Ho_Chi_Minh", "the timezone"},
		{"php_admin_value[open_basedir] = /vkai-panel/www/domains/example.com:/tmp", "the site confinement"},
		{"php_admin_value[disable_functions] = exec,system", "disabled functions, sorted so the file is stable"},
		{"php_admin_flag[display_errors] = off", "errors must not be shown to visitors by default"},
		{";   extension = imagick", "the requested extension set is recorded in the pool file"},
		{";   extension = redis", "the requested extension set is recorded in the pool file"},
	}
	for _, want := range required {
		if !strings.Contains(text, want.line) {
			t.Errorf("the pool file does not contain %q: %s\n---\n%s", want.line, want.why, text)
		}
	}

	// php_value would let the site raise the limit back. It must not appear.
	if strings.Contains(text, "php_value[memory_limit]") {
		t.Error("memory_limit was written as php_value, which a site can override with ini_set(); " +
			"it must be php_admin_value")
	}
}

// TestRenderIsDeterministic proves two renders of the same spec are byte
// identical. A pool file that changes on every apply makes every settings save
// look like a change, and a rollback impossible to verify by comparison.
func TestRenderIsDeterministic(t *testing.T) {
	first, err := validSpec().Render()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		again, err := validSpec().Render()
		if err != nil {
			t.Fatal(err)
		}
		if string(first) != string(again) {
			t.Fatal("two renders of the same pool spec differ; map iteration order is leaking " +
				"into the pool file")
		}
	}
}

// TestPoolValidationRefusesInjectionAndNonsense is the security half. A pool
// file has no quoting: a newline in a value is a second directive, and the
// second directive an attacker wants is `user = root`.
func TestPoolValidationRefusesInjectionAndNonsense(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PoolSpec)
		want   string
	}{
		{"a newline in the pool name becomes a second directive", func(s *PoolSpec) {
			s.Name = "site\nuser = root"
		}, "invalid pool name"},
		{"a path traversal in the pool name escapes the pool directory", func(s *PoolSpec) {
			s.Name = "../../../etc/cron.d/evil"
		}, "invalid pool name"},
		{"a pool running as root gives every request full privileges", func(s *PoolSpec) {
			s.User = "root"
		}, "may not run as root"},
		{"a pool in the root group", func(s *PoolSpec) {
			s.Group = "root"
		}, "may not run in the root group"},
		{"a newline in the user name", func(s *PoolSpec) {
			s.User = "site\nuser = root"
		}, "invalid pool user"},
		{"a version that is a path", func(s *PoolSpec) {
			s.Version = "../../etc"
		}, "invalid php version"},
		{"a version outside the supported range", func(s *PoolSpec) {
			s.Version = "5.6"
		}, "outside the supported range"},
		{"a memory limit that is a directive", func(s *PoolSpec) {
			s.MemoryLimit = "512M\nuser = root"
		}, "invalid memory_limit"},
		{"a memory limit with the wrong unit", func(s *PoolSpec) {
			s.MemoryLimit = "512MB"
		}, "invalid memory_limit"},
		{"an upload size that is a directive", func(s *PoolSpec) {
			s.UploadMaxFilesize = "64M\nphp_admin_value[open_basedir] = /"
		}, "invalid upload_max_filesize"},
		{"a negative execution time", func(s *PoolSpec) {
			s.MaxExecutionTime = -5
		}, "max_execution_time must be between"},
		{"an absurd execution time", func(s *PoolSpec) {
			s.MaxExecutionTime = 999999
		}, "max_execution_time must be between"},
		{"an extension name that is a shell fragment", func(s *PoolSpec) {
			s.Extensions = []string{"redis; rm -rf /"}
		}, "invalid extension name"},
		{"an extension name that is a package flag", func(s *PoolSpec) {
			s.Extensions = []string{"--allow-downgrade"}
		}, "invalid extension name"},
		{"an unknown process manager", func(s *PoolSpec) {
			s.PM = "aggressive"
		}, "invalid process manager"},
		{"start_servers outside the spare range, which php-fpm refuses to start on", func(s *PoolSpec) {
			s.PMStartServers = 99
		}, "php-fpm refuses to start otherwise"},
		{"max_spare_servers above max_children", func(s *PoolSpec) {
			s.PMMaxSpareServers = 999
			s.PMStartServers = 500
		}, "must not exceed pm.max_children"},
		{"a relative error log path", func(s *PoolSpec) {
			s.ErrorLog = "../../etc/passwd"
		}, "must be absolute"},
		{"a traversal in open_basedir", func(s *PoolSpec) {
			s.OpenBasedir = []string{"/vkai-panel/www/../../etc"}
		}, "must not contain .."},
		{"a newline in an environment variable value", func(s *PoolSpec) {
			s.Env = map[string]string{"APP": "x\nuser = root"}
		}, "would become a second directive"},
		{"a listen mode that is not octal", func(s *PoolSpec) {
			s.ListenMode = "0999"
		}, "invalid listen mode"},
		{"a function name that is a directive", func(s *PoolSpec) {
			s.DisabledFunctions = []string{"exec\nuser = root"}
		}, "invalid function name"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec()
			tc.mutate(spec)

			if _, err := spec.Render(); err == nil {
				t.Fatalf("Render accepted it; it must be refused because %s", tc.name)
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refused with %q, want a message containing %q", err, tc.want)
			}

			// The same value must also be refused by Validate on its own, so a
			// caller that validates before rendering gets the same answer.
			if err := spec.Validate(); err == nil {
				t.Fatal("Validate accepted what Render refused")
			}
		})
	}
}

// TestRenderedFileNeverContainsARootDirective is the belt-and-braces assertion:
// whatever else changes, no rendered pool file may end up saying user = root.
func TestRenderedFileNeverContainsARootDirective(t *testing.T) {
	spec := validSpec()
	rendered, err := spec.Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(rendered), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ";") {
			continue
		}
		if trimmed == "user = root" || trimmed == "group = root" {
			t.Fatalf("the rendered pool file contains %q", trimmed)
		}
	}
}

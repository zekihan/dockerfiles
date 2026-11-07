package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"go.yaml.in/yaml/v3"
)

const sourcesYAML = `---
source: victorialogs
mode: tail
url: http://configured-by-eso.invalid/
query: "cluster:=\"cluster-base\" kubernetes.namespace_name:=\"traefik\""
wait_for_ready: 10s
max_failure_duration: 5m
labels:
  type: traefik
---
source: victorialogs
mode: tail
url: http://configured-by-eso.invalid/
headers:
  Stale: drop-me
query: "_SYSTEMD_UNIT:=\"ssh.service\""
wait_for_ready: 10s
max_failure_duration: 5m
labels:
  type: sshd
`

const connectionYAML = `url: http://fixture:9428/
headers:
  AccountID: "0"
  ProjectID: 0
auth:
  username: vl
  password: secret
`

const credentials = "url: http://lapi:8080/\nlogin: test\npassword: disposable\n"

func write(t *testing.T, root, path, data string, mode os.FileMode) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	os.Remove(full)
	if err := os.WriteFile(full, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, path string) (string, os.FileMode) {
	t.Helper()
	full := filepath.Join(root, path)
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), info.Mode().Perm()
}

func agentFixture(t *testing.T) string {
	root := t.TempDir()
	write(t, root, "/tmp_config/local_api_credentials.yaml", credentials, 0o644)
	write(t, root, "/staging/etc/crowdsec/config.yaml", "common: {}\n", 0o644)
	write(t, root, "/connection/connection.yaml", connectionYAML, 0o400)
	write(t, root, "/runtime/sources.yaml", sourcesYAML, 0o555)
	return root
}

func withUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(old) })
}

func TestPrepareAgent(t *testing.T) {
	withUmask(t)
	root := agentFixture(t)
	if err := prepareAgent(fsRoot(root)); err != nil {
		t.Fatal(err)
	}
	acquis, mode := read(t, root, "/tmp_config/victorialogs-acquis.yaml")
	want := `---
source: victorialogs
mode: tail
query: "cluster:=\"cluster-base\" kubernetes.namespace_name:=\"traefik\""
wait_for_ready: 10s
max_failure_duration: 5m
labels:
  type: traefik
url: http://fixture:9428/
headers:
  AccountID: "0"
  ProjectID: 0
auth:
  username: vl
  password: secret
---
source: victorialogs
mode: tail
query: "_SYSTEMD_UNIT:=\"ssh.service\""
wait_for_ready: 10s
max_failure_duration: 5m
labels:
  type: sshd
url: http://fixture:9428/
headers:
  AccountID: "0"
  ProjectID: 0
auth:
  username: vl
  password: secret
`
	if acquis != want || mode != 0o600 {
		t.Fatalf("acquis mode %o:\n%s", mode, acquis)
	}
	health, mode := read(t, root, "/tmp_config/health-sources.jsonl")
	wantHealth := `{"source":"victorialogs","mode":"tail","query":"cluster:=\"cluster-base\" kubernetes.namespace_name:=\"traefik\"","wait_for_ready":"10s","max_failure_duration":"5m","labels":{"type":"traefik"},"url":"http://fixture:9428/","headers":{"AccountID":"0","ProjectID":0},"auth":{"username":"vl","password":"secret"}}
{"source":"victorialogs","mode":"tail","query":"_SYSTEMD_UNIT:=\"ssh.service\"","wait_for_ready":"10s","max_failure_duration":"5m","labels":{"type":"sshd"},"url":"http://fixture:9428/","headers":{"AccountID":"0","ProjectID":0},"auth":{"username":"vl","password":"secret"}}
`
	if health != wantHealth || mode != 0o600 {
		t.Fatalf("health mode %o:\n%s", mode, health)
	}
	sources, err := loadSources(filepath.Join(root, "/tmp_config/health-sources.jsonl"))
	if err != nil || len(sources) != 2 || sources[0].Headers["ProjectID"] != "0" {
		t.Fatalf("health sources unreadable: %+v %v", sources, err)
	}
	creds, mode := read(t, root, "/state/local_api_credentials.yaml")
	if creds != credentials || mode != 0o600 {
		t.Fatalf("credentials mode %o: %q", mode, creds)
	}
	config, mode := read(t, root, "/state/config.yaml")
	if config != "common: {}\n" || mode != 0o600 {
		t.Fatalf("config mode %o: %q", mode, config)
	}
	for _, dir := range []string{"/state", "/tmp_config/acquis.d"} {
		info, err := os.Stat(filepath.Join(root, dir))
		if err != nil || !info.IsDir() {
			t.Fatalf("%s: %v", dir, err)
		}
	}
	info, _ := os.Stat(filepath.Join(root, "/tmp_config/acquis.d"))
	if info.Mode().Perm() != 0o700 {
		t.Errorf("acquis.d mode %o", info.Mode().Perm())
	}
}

func TestPrepareAgentKeepsExistingConfigAndRefreshesCredentials(t *testing.T) {
	withUmask(t)
	root := agentFixture(t)
	write(t, root, "/state/config.yaml", "edited: true\n", 0o640)
	write(t, root, "/state/local_api_credentials.yaml", "old\n", 0o644)
	if err := prepareAgent(fsRoot(root)); err != nil {
		t.Fatal(err)
	}
	if config, _ := read(t, root, "/state/config.yaml"); config != "edited: true\n" {
		t.Fatalf("config overwritten: %q", config)
	}
	if creds, mode := read(t, root, "/state/local_api_credentials.yaml"); creds != credentials || mode != 0o600 {
		t.Fatalf("credentials not refreshed: %o %q", mode, creds)
	}
}

func TestPrepareAgentFailsClosed(t *testing.T) {
	cases := []struct {
		name, path, data, err string
	}{
		{"missing credentials", "/tmp_config/local_api_credentials.yaml", "", "no such file"},
		{"empty credentials", "/tmp_config/local_api_credentials.yaml", "\n", "empty credentials"},
		{"null password", "/tmp_config/local_api_credentials.yaml", "login: a\npassword: null\n", "missing password"},
		{"no login", "/tmp_config/local_api_credentials.yaml", "password: a\n", "missing login"},
		{"scalar credentials", "/tmp_config/local_api_credentials.yaml", "nope\n", "must be a mapping"},
		{"broken credentials", "/tmp_config/local_api_credentials.yaml", "login: [\n", "yaml"},
		{"missing connection", "/connection/connection.yaml", "", "no such file"},
		{"extra connection key", "/connection/connection.yaml", "url: http://x/\nquery: '*'\n", `unexpected key "query"`},
		{"connection without url", "/connection/connection.yaml", "headers: {a: b}\n", "url must be"},
		{"non-http url", "/connection/connection.yaml", "url: ftp://x/\n", "url must be"},
		{"uppercase scheme", "/connection/connection.yaml", "url: HTTP://x/\n", "url must be"},
		{"numeric url", "/connection/connection.yaml", "url: 5\n", "url must be"},
		{"headers not a map", "/connection/connection.yaml", "url: http://x/\nheaders: a\n", "headers must be a mapping"},
		{"two connection documents", "/connection/connection.yaml", "url: http://x/\n---\nurl: http://y/\n", "exactly one"},
		{"empty connection", "/connection/connection.yaml", "", "exactly one"},
		{"null connection", "/connection/connection.yaml", "null\n", "must be a mapping"},
		{"missing sources", "/runtime/sources.yaml", "", "no such file"},
		{"empty sources", "/runtime/sources.yaml", "", "no acquisition sources"},
		{"scalar source", "/runtime/sources.yaml", "---\nfoo\n", "must be a mapping"},
		{"broken sources", "/runtime/sources.yaml", "a: [\n", "yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withUmask(t)
			root := agentFixture(t)
			full := filepath.Join(root, tc.path)
			if tc.err == "no such file" {
				os.Remove(full)
			} else {
				write(t, root, tc.path, tc.data, 0o600)
			}
			err := prepareAgent(fsRoot(root))
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("err = %v, want %q", err, tc.err)
			}
			for _, out := range []string{"/state/local_api_credentials.yaml", "/tmp_config/victorialogs-acquis.yaml", "/tmp_config/health-sources.jsonl"} {
				if _, err := os.Stat(filepath.Join(root, out)); err == nil {
					t.Errorf("%s written despite failure", out)
				}
			}
		})
	}
}

func TestMergeIsDeep(t *testing.T) {
	root := agentFixture(t)
	write(t, root, "/runtime/sources.yaml", "labels: {type: a}\nnested: {keep: 1, over: 1}\nlist: [1, 2]\n", 0o600)
	write(t, root, "/connection/connection.yaml", "url: https://vl/\n", 0o600)
	withUmask(t)
	if err := prepareAgent(fsRoot(root)); err != nil {
		t.Fatal(err)
	}
	got, _ := read(t, root, "/tmp_config/health-sources.jsonl")
	if got != `{"labels":{"type":"a"},"nested":{"keep":1,"over":1},"list":[1,2],"url":"https://vl/"}`+"\n" {
		t.Fatal(got)
	}
	dst := mapping(mustDoc(t, "headers: {a: 1, b: {c: 1}, l: [1]}\nkeep: x\n"))
	src := mapping(mustDoc(t, "headers: {b: {d: 2}, e: 3, l: [2]}\n"))
	out, err := encodeJSONLines([]*yaml.Node{merge(dst, src)})
	if err != nil || string(out) != `{"headers":{"a":1,"b":{"c":1,"d":2},"l":[2],"e":3},"keep":"x"}`+"\n" {
		t.Fatalf("merge: %s %v", out, err)
	}
}

func mustDoc(t *testing.T, data string) *yaml.Node {
	t.Helper()
	docs, err := decodeDocuments([]byte(data))
	if err != nil || len(docs) != 1 {
		t.Fatalf("%v %d", err, len(docs))
	}
	return docs[0]
}

func TestPrepareLAPI(t *testing.T) {
	withUmask(t)
	root := t.TempDir()
	write(t, root, "/staging/etc/crowdsec/config.yaml", "common: {}\n", 0o644)
	write(t, root, "/credentials/local_api_credentials.yaml", credentials, 0o400)
	write(t, root, "/credentials/online_api_credentials.yaml", "url: https://api.crowdsec.net/\nlogin: o\npassword: p\n", 0o400)
	if err := prepareLAPI(fsRoot(root)); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"/state/local_api_credentials.yaml":  credentials,
		"/state/online_api_credentials.yaml": "url: https://api.crowdsec.net/\nlogin: o\npassword: p\n",
		"/state/config.yaml":                 "common: {}\n",
	} {
		if got, mode := read(t, root, name); got != want || mode != 0o600 {
			t.Errorf("%s mode %o: %q", name, mode, got)
		}
	}
	info, err := os.Stat(filepath.Join(root, "/state/notifications"))
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("notifications: %v %v", info, err)
	}
}

func TestPrepareLAPIFailsClosed(t *testing.T) {
	for _, tc := range []struct{ name, file, data string }{
		{"local missing url", "local_api_credentials.yaml", "login: a\npassword: b\n"},
		{"online missing password", "online_api_credentials.yaml", "url: x\nlogin: a\n"},
		{"online missing", "online_api_credentials.yaml", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withUmask(t)
			root := t.TempDir()
			write(t, root, "/staging/etc/crowdsec/config.yaml", "common: {}\n", 0o644)
			write(t, root, "/credentials/local_api_credentials.yaml", credentials, 0o400)
			write(t, root, "/credentials/online_api_credentials.yaml", credentials, 0o400)
			if tc.data == "" {
				os.Remove(filepath.Join(root, "/credentials", tc.file))
			} else {
				write(t, root, "/credentials/"+tc.file, tc.data, 0o400)
			}
			if err := prepareLAPI(fsRoot(root)); err == nil {
				t.Fatal("accepted invalid credentials")
			}
			if _, err := os.Stat(filepath.Join(root, "/state/local_api_credentials.yaml")); err == nil {
				t.Error("credentials written despite failure")
			}
		})
	}
}

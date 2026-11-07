package main

// Go versions of the homelab prepare-agent.sh and prepare-lapi.sh init scripts.
// Every input is validated before anything is written, and failures exit
// non-zero so the Pod never starts with missing or malformed credentials.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"

	"go.yaml.in/yaml/v3"
)

var httpURL = regexp.MustCompile(`^https?://`)

// fsRoot prefixes every absolute path; "/" in the image, a temp dir in tests.
type fsRoot string

func (r fsRoot) path(p string) string { return filepath.Join(string(r), p) }

func decodeDocuments(data []byte) ([]*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, &doc)
	}
}

func readDocuments(path string) ([]*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	docs, err := decodeDocuments(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return docs, nil
}

func mapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		doc = doc.Content[0]
	}
	if doc.Kind == yaml.MappingNode {
		return doc
	}
	return nil
}

func lookup(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func isNull(n *yaml.Node) bool {
	return n == nil || (n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null")
}

func remove(m *yaml.Node, keys ...string) {
	drop := map[string]bool{}
	for _, k := range keys {
		drop[k] = true
	}
	content := m.Content[:0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if !drop[m.Content[i].Value] {
			content = append(content, m.Content[i], m.Content[i+1])
		}
	}
	m.Content = content
}

// checkCredentials mirrors `yq -e '.a != null and .b != null' file`.
func checkCredentials(path string, keys ...string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	docs, err := decodeDocuments(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("%s: empty credentials file", path)
	}
	for _, doc := range docs {
		m := mapping(doc)
		if m == nil {
			return nil, fmt.Errorf("%s: credentials must be a mapping", path)
		}
		for _, key := range keys {
			if isNull(lookup(m, key)) {
				return nil, fmt.Errorf("%s: missing %s", path, key)
			}
		}
	}
	return data, nil
}

// loadConnection enforces the connection.yaml contract: one mapping holding
// only url, headers and auth, with an http(s) url.
func loadConnection(path string) (*yaml.Node, error) {
	docs, err := readDocuments(path)
	if err != nil {
		return nil, err
	}
	if len(docs) != 1 {
		return nil, fmt.Errorf("%s: expected exactly one YAML document, got %d", path, len(docs))
	}
	m := mapping(docs[0])
	if m == nil {
		return nil, fmt.Errorf("%s: connection must be a mapping", path)
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		switch key := m.Content[i].Value; key {
		case "url":
		case "headers", "auth":
			if m.Content[i+1].Kind != yaml.MappingNode {
				return nil, fmt.Errorf("%s: %s must be a mapping", path, key)
			}
		default:
			return nil, fmt.Errorf("%s: unexpected key %q (only url, headers and auth are allowed)", path, key)
		}
	}
	u := lookup(m, "url")
	if u == nil || u.Kind != yaml.ScalarNode || u.ShortTag() != "!!str" || !httpURL.MatchString(u.Value) {
		return nil, fmt.Errorf("%s: url must be an http:// or https:// string", path)
	}
	return m, nil
}

// merge is yq's `*`: maps merge recursively, anything else is replaced by src.
func merge(dst, src *yaml.Node) *yaml.Node {
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		return clone(src)
	}
	for i := 0; i+1 < len(src.Content); i += 2 {
		key, value := src.Content[i], src.Content[i+1]
		found := false
		for j := 0; j+1 < len(dst.Content); j += 2 {
			if dst.Content[j].Value == key.Value {
				dst.Content[j+1] = merge(dst.Content[j+1], value)
				found = true
				break
			}
		}
		if !found {
			dst.Content = append(dst.Content, clone(key), clone(value))
		}
	}
	return dst
}

// clone copies merged-in connection values; like yq they are emitted in block style.
func clone(n *yaml.Node) *yaml.Node {
	c := *n
	c.Style &^= yaml.FlowStyle
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, child := range n.Content {
		c.Content[i] = clone(child)
	}
	return &c
}

// explicitStart reports whether a stream opens with "---", which yq keeps.
func explicitStart(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		return bytes.Equal(line, []byte("---")) || bytes.HasPrefix(line, []byte("--- "))
	}
	return false
}

func renderSources(sourcesPath string, connection *yaml.Node) ([]*yaml.Node, bool, error) {
	data, err := os.ReadFile(sourcesPath)
	if err != nil {
		return nil, false, err
	}
	docs, err := decodeDocuments(data)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", sourcesPath, err)
	}
	var out []*yaml.Node
	for n, doc := range docs {
		m := mapping(doc)
		if m == nil {
			return nil, false, fmt.Errorf("%s: document %d must be a mapping", sourcesPath, n+1)
		}
		remove(m, "url", "headers", "auth")
		out = append(out, merge(m, connection))
	}
	if len(out) == 0 {
		return nil, false, fmt.Errorf("%s: no acquisition sources", sourcesPath)
	}
	return out, explicitStart(data), nil
}

func encodeYAML(docs []*yaml.Node, start bool) ([]byte, error) {
	var buf bytes.Buffer
	if start {
		buf.WriteString("---\n")
	}
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	for _, doc := range docs {
		if err := encoder.Encode(doc); err != nil {
			return nil, err
		}
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// toJSON writes a node as compact JSON, keeping mapping key order like yq.
func toJSON(buf *bytes.Buffer, n *yaml.Node) error {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) != 1 {
			return errors.New("empty document")
		}
		return toJSON(buf, n.Content[0])
	case yaml.AliasNode:
		return toJSON(buf, n.Alias)
	case yaml.MappingNode:
		buf.WriteByte('{')
		for i := 0; i+1 < len(n.Content); i += 2 {
			if i > 0 {
				buf.WriteByte(',')
			}
			key, _ := json.Marshal(n.Content[i].Value)
			buf.Write(key)
			buf.WriteByte(':')
			if err := toJSON(buf, n.Content[i+1]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case yaml.SequenceNode:
		buf.WriteByte('[')
		for i, item := range n.Content {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := toJSON(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case yaml.ScalarNode:
		var value any = n.Value
		switch n.ShortTag() {
		case "!!null":
			value = nil
		case "!!bool", "!!float":
			if err := n.Decode(&value); err != nil {
				return err
			}
		case "!!int":
			var i int64
			if err := n.Decode(&i); err != nil {
				return err
			}
			value = json.Number(strconv.FormatInt(i, 10))
		}
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("value %q: %w", n.Value, err)
		}
		buf.Write(data)
	default:
		return fmt.Errorf("unsupported YAML node kind %d", n.Kind)
	}
	return nil
}

func encodeJSONLines(docs []*yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	for _, doc := range docs {
		if err := toJSON(&buf, doc); err != nil {
			return nil, err
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// writeFile atomically replaces path with data at mode 0600.
func writeFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// seedConfig copies the staged config.yaml once, like `[ -f ] || cp`.
func seedConfig(root fsRoot) error {
	target := root.path("/state/config.yaml")
	info, err := os.Stat(target)
	switch {
	case err == nil && info.Mode().IsRegular():
		return nil
	case err == nil:
		return fmt.Errorf("%s exists and is not a regular file", target)
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	source := root.path("/staging/etc/crowdsec/config.yaml")
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return writeFile(target, data)
}

func prepareAgent(root fsRoot) error {
	credentials, err := checkCredentials(root.path("/tmp_config/local_api_credentials.yaml"), "login", "password")
	if err != nil {
		return err
	}
	connection, err := loadConnection(root.path("/connection/connection.yaml"))
	if err != nil {
		return err
	}
	sources, start, err := renderSources(root.path("/runtime/sources.yaml"), connection)
	if err != nil {
		return err
	}
	acquis, err := encodeYAML(sources, start)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(acquis)) == 0 {
		return errors.New("victorialogs-acquis.yaml would be empty")
	}
	health, err := encodeJSONLines(sources)
	if err != nil {
		return fmt.Errorf("health-sources.jsonl: %w", err)
	}
	for _, dir := range []string{"/state", "/tmp_config/acquis.d"} {
		if err := os.MkdirAll(root.path(dir), 0o777); err != nil {
			return err
		}
	}
	if err := writeFile(root.path("/state/local_api_credentials.yaml"), credentials); err != nil {
		return err
	}
	if err := seedConfig(root); err != nil {
		return err
	}
	if err := writeFile(root.path("/tmp_config/victorialogs-acquis.yaml"), acquis); err != nil {
		return err
	}
	return writeFile(root.path("/tmp_config/health-sources.jsonl"), health)
}

func prepareLAPI(root fsRoot) error {
	names := []string{"local_api_credentials.yaml", "online_api_credentials.yaml"}
	files := map[string][]byte{}
	for _, name := range names {
		data, err := checkCredentials(root.path("/credentials/"+name), "url", "login", "password")
		if err != nil {
			return err
		}
		files[name] = data
	}
	if err := os.MkdirAll(root.path("/state/notifications"), 0o777); err != nil {
		return err
	}
	if err := seedConfig(root); err != nil {
		return err
	}
	for _, name := range names {
		if err := writeFile(root.path("/state/"+name), files[name]); err != nil {
			return err
		}
	}
	return nil
}

func runPrepare(name string, f func(fsRoot) error) int {
	syscall.Umask(0o077)
	if err := f(fsRoot("/")); err != nil {
		fmt.Fprintf(os.Stderr, "crowdsec-supervisor %s: %v\n", name, err)
		return 1
	}
	return 0
}

package agentenv

import "testing"

func TestValues(t *testing.T) {
	values := Values("127.0.0.1:17890")
	for _, name := range []string{
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"ALL_PROXY",
		"http_proxy",
		"https_proxy",
		"all_proxy",
	} {
		if values[name] != "http://127.0.0.1:17890" {
			t.Fatalf("%s = %q", name, values[name])
		}
	}
	if values["NO_PROXY"] != "localhost,127.0.0.1,::1" {
		t.Fatalf("NO_PROXY = %q", values["NO_PROXY"])
	}
}

func TestPreviousSnapshot(t *testing.T) {
	previous := previousSnapshot(
		map[string]string{"HTTP_PROXY": "http://old"},
		map[string]string{"HTTP_PROXY": "http://new", "HTTPS_PROXY": "http://new"},
	)
	if previous["HTTP_PROXY"] == nil || *previous["HTTP_PROXY"] != "http://old" {
		t.Fatalf("HTTP_PROXY previous = %#v", previous["HTTP_PROXY"])
	}
	if previous["HTTPS_PROXY"] != nil {
		t.Fatalf("HTTPS_PROXY previous = %#v, want nil", previous["HTTPS_PROXY"])
	}
}

package permission

import "testing"

// TestGuardKey: the names that reach one file on some file system share
// a key, and names that are plainly other files do not.
func TestGuardKey(t *testing.T) {
	same := [][]string{
		{".env", ".ENV", ".Env", ".env.", ".env ", ".env. .", ".e\u200cnv", "\ufeff.env", ".e\u202env"},
		{"caf\u00e9", "cafe\u0301", "CAFE\u0301"},
		{"secrets", "\u017fecrets", "SECRETS", "secret\u017f"},
		{"kelvin", "\u212aelvin"},
		{"strasse", "stra\u00dfe", "STRA\u1e9eE"},
		{"config", "con\ufb01g"},
		{"\ube44\ubc00", "\u1107\u1175\u1106\u1175\u11af"},
		{"\u4e2d\u6587", "\u4e2d\u6587."},
	}
	for _, group := range same {
		for _, name := range group[1:] {
			if guardKey(name) != guardKey(group[0]) {
				t.Errorf("%q and %q have the keys %q and %q, want one key", group[0], name, guardKey(group[0]), guardKey(name))
			}
		}
	}
	differ := [][2]string{
		{".env", "env"}, {".env", ".envrc"}, {"cafe", "caf\u00e9"}, {"cafe", "cafe\u0301"}, {"resume", "r\u00e9sum\u00e9"},
		{"\ube44\ubc00", "\ubb38\uc11c"}, {"\u4e2d\u6587", "\u65e5\u672c"}, {"a.b", "ab"}, {"a b", "ab"},
		{"\u0441\u0435\u043a", "\u0441\u0435\u043a\u0440"}, {"x", "\u0445"},
	}
	for _, pair := range differ {
		if guardKey(pair[0]) == guardKey(pair[1]) {
			t.Errorf("%q and %q share the key %q", pair[0], pair[1], guardKey(pair[0]))
		}
	}
	// A name that is all dots or blanks keeps a key.
	for _, name := range []string{".", "..", " ", "...", ". ."} {
		if guardKey(name) == "" {
			t.Errorf("%q has no key", name)
		}
	}
	if got := guardPath("/Proj/Sub Dir/.ENV."); got != "/proj/sub dir/.env" {
		t.Errorf("guardPath: %q", got)
	}
}

// TestCanonHost: one form for every way of writing a host.
func TestCanonHost(t *testing.T) {
	for in, want := range map[string]string{
		"example.com": "example.com", "EXAMPLE.Com.": "example.com", "example.com..": "example.com",
		"127.0.0.1": "127.0.0.1", "127.1": "127.0.0.1", "127.0.1": "127.0.0.1", "2130706433": "127.0.0.1",
		"0x7f.0.0.1": "127.0.0.1", "0X7F000001": "127.0.0.1", "0177.0.0.1": "127.0.0.1", "017700000001": "127.0.0.1",
		"0": "0.0.0.0", "1.2.3.4": "1.2.3.4", "1.2.3.": "1.2.0.3", "10.0x0.0.1": "10.0.0.1", "192.168.257": "192.168.1.1",
		"::1": "::1", "0:0:0:0:0:0:0:1": "::1", "::ffff:127.0.0.1": "127.0.0.1", "::FFFF:7f00:1": "127.0.0.1",
		"2001:DB8::1": "2001:db8::1", "fe80::1%eth0": "fe80::1%eth0", "xn--exmple-cua.com": "xn--exmple-cua.com",
		"1password.example": "1password.example", "a_b.example": "a_b.example", "localhost": "localhost",
	} {
		if got, ok := canonHost(in); !ok || got != want {
			t.Errorf("canonHost(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"", ".", "ex\u00e4mple.com", "example\u3002com", "a b", "a/b", "a@b", "a\\b", "08.1.1.1", "1.2.3.4.5", "256.1.1.1",
		"1.2.3.256", "4294967296", "0x", "::g", "1::2::3", "a%2eb", "[::1]", "a?b", "a#b",
	} {
		if got, ok := canonHost(in); ok {
			t.Errorf("canonHost(%q) = %q, want it refused", in, got)
		}
	}
}

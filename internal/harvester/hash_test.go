package harvester

import "testing"

func TestProjectDirName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/", "-"},
		{"/home/joncik", "-home-joncik"},
		{"/home/joncik/apps/inflate", "-home-joncik-apps-inflate"},
		{"/root/apps/aaOS", "-root-apps-aaOS"},
		{"/home/joncik/apps/Codexbar-fork", "-home-joncik-apps-Codexbar-fork"},
	}
	for _, c := range cases {
		got := ProjectDirName(c.in)
		if got != c.want {
			t.Errorf("ProjectDirName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestProjectDirNameSpacesAndPunctuation pins the broader behavior derived
// from the empirical Claude Code mapping (spaces collapse to "-").
func TestProjectDirNameSpacesAndPunctuation(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/home/joncik/Projects/starting from scratch", "-home-joncik-Projects-starting-from-scratch"},
		{"/a/b c/d  e", "-a-b-c-d-e"},
		{"/foo.bar/baz_qux", "-foo-bar-baz-qux"},
	}
	for _, c := range cases {
		got := ProjectDirName(c.in)
		if got != c.want {
			t.Errorf("ProjectDirName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

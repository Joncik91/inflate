package harvester

import "testing"

func TestProjectDirName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/home/joncik/apps/inflate", "-home-joncik-apps-inflate"},
		{"/root/apps/aaOS", "-root-apps-aaOS"},
		{"/home/joncik", "-home-joncik"},
		{"/", "-"},
	}
	for _, c := range cases {
		got := ProjectDirName(c.in)
		if got != c.want {
			t.Errorf("ProjectDirName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

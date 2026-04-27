package harvester

import "os"

func userHomeDir() (string, error) { return os.UserHomeDir() }

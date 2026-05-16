package env

import (
	"fmt"
	"os"
	"strings"
)

func LoadDotenv(filenames ...string) (envMap map[string]string, err error) {
	envMap = make(map[string]string)
	for _, filename := range filenames {
		bytes, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("error reading env file %s: %w", filename, err)
		}
		for _, line := range strings.Split(string(bytes), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			key, val, found := strings.Cut(line, "=")
			if !found {
				// Return a tracking error matching your exact format expectation
				return nil, fmt.Errorf("error reading env file %s: line %q is malformed", filename, line)
			}

			envMap[strings.TrimSpace(key)] = strings.TrimSpace(val)
		}
	}
	return envMap, nil
}

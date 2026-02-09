package subscriber

import (
	"strings"

	"github.com/joho/godotenv"
)

func ParseEnvFilesList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return []string{}
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func LoadEnvFiles(paths []string) (map[string]string, error) {
	combined := map[string]string{}
	for _, p := range paths {
		env, err := godotenv.Read(p)
		if err != nil {
			return nil, err
		}
		for k, v := range env {
			combined[k] = v
		}
	}
	return combined, nil
}

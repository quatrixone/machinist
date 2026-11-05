package controlplane

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func readHermesPublicCatalog(home, provider string) ([]string, error) {
	body, err := os.ReadFile(filepath.Join(home, "models_dev_cache.json"))
	if err != nil {
		return nil, err
	}
	var catalog map[string]struct {
		Models map[string]struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err = json.Unmarshal(body, &catalog); err != nil {
		return nil, err
	}
	models, ok := catalog[provider]
	if !ok {
		return nil, os.ErrNotExist
	}
	ids := make([]string, 0, len(models.Models))
	for id := range models.Models {
		id = strings.TrimSpace(id)
		if id != "" && len(id) <= maxModelIDLength && !strings.ContainsAny(id, "\x00\r\n") {
			ids = append(ids, provider+"/"+id)
		}
		if len(ids) >= maxAdvertisedModels {
			break
		}
	}
	sort.Strings(ids)
	return ids, nil
}

package rpg

import (
	"context"
	"fmt"
	"log"

	"github.com/yoanbernabeu/grepai/config"
)

// ProjectRPGStore holds a per-project RPG store and query engine.
type ProjectRPGStore struct {
	ProjectName string
	ProjectPath string
	Store       RPGStore
	QE          *QueryEngine
}

// LoadWorkspaceRPGStores loads GOB RPG stores for workspace projects.
// If projectName is non-empty, only that project's store is loaded.
// Unlike symbol stores, RPG is optional enrichment — projects that fail to load
// or have RPG disabled are silently skipped.
func LoadWorkspaceRPGStores(ctx context.Context, workspaceName, projectName string) ([]ProjectRPGStore, error) {
	wsCfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace config: %w", err)
	}
	if wsCfg == nil {
		return nil, fmt.Errorf("no workspaces configured; create one with: grepai workspace create <name>")
	}

	ws, err := wsCfg.GetWorkspace(workspaceName)
	if err != nil {
		return nil, err
	}

	var projects []config.ProjectEntry
	if projectName != "" {
		found := false
		for _, p := range ws.Projects {
			if p.Name == projectName {
				projects = []config.ProjectEntry{p}
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("project %q not found in workspace %q", projectName, workspaceName)
		}
	} else {
		projects = ws.Projects
	}

	stores := make([]ProjectRPGStore, 0, len(projects))
	for _, p := range projects {
		// Load per-project config to check if RPG is enabled
		cfg, cfgErr := config.Load(p.Path)
		if cfgErr != nil {
			log.Printf("Warning: skipping RPG for project %s: failed to load config: %v", p.Name, cfgErr)
			continue
		}
		if !cfg.RPG.Enabled {
			continue
		}

		rpgStore := NewGOBRPGStore(config.GetRPGIndexPath(p.Path))
		if loadErr := rpgStore.Load(ctx); loadErr != nil {
			log.Printf("Warning: skipping RPG for project %s: %v", p.Name, loadErr)
			rpgStore.Close()
			continue
		}

		graph := rpgStore.GetGraph()
		if graph.Stats().TotalNodes == 0 {
			rpgStore.Close()
			continue
		}

		qe := NewQueryEngine(graph)
		stores = append(stores, ProjectRPGStore{
			ProjectName: p.Name,
			ProjectPath: p.Path,
			Store:       rpgStore,
			QE:          qe,
		})
	}
	return stores, nil
}

// CloseRPGStores closes all RPG stores in the slice.
func CloseRPGStores(stores []ProjectRPGStore) {
	for _, s := range stores {
		s.Store.Close()
	}
}

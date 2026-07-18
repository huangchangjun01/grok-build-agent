package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// HookFunc is a function that can be registered as a hook.
type HookFunc func(ctx context.Context, data map[string]interface{}) error

// Plugin represents a loadable plugin.
type Plugin struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Version     string              `json:"version"`
	Hooks       map[string]HookFunc `json:"-"`
}

// PluginManager manages plugins.
type PluginManager struct {
	plugins map[string]*Plugin
	hooks   map[string][]HookFunc // hookName -> []HookFunc
	logger  *logrus.Logger
	mu      sync.RWMutex
}

// NewPluginManager creates a new PluginManager.
func NewPluginManager(logger *logrus.Logger) *PluginManager {
	return &PluginManager{
		plugins: make(map[string]*Plugin),
		hooks:   make(map[string][]HookFunc),
		logger:  logger,
	}
}

// RegisterPlugin registers a plugin.
func (m *PluginManager) RegisterPlugin(plugin *Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if plugin.Name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}

	if _, exists := m.plugins[plugin.Name]; exists {
		return fmt.Errorf("plugin %q is already registered", plugin.Name)
	}

	m.plugins[plugin.Name] = plugin

	// Register the plugin's hooks
	for hookName, fn := range plugin.Hooks {
		m.hooks[hookName] = append(m.hooks[hookName], fn)
		m.logger.WithFields(logrus.Fields{
			"plugin":   plugin.Name,
			"hook":     hookName,
			"version":  plugin.Version,
		}).Info("plugin hook registered")
	}

	m.logger.WithFields(logrus.Fields{
		"plugin":  plugin.Name,
		"version": plugin.Version,
	}).Info("plugin registered")

	return nil
}

// UnregisterPlugin unregisters a plugin.
func (m *PluginManager) UnregisterPlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %q is not registered", name)
	}

	// Remove the plugin's hooks
	for hookName := range plugin.Hooks {
		m.removePluginHooks(hookName, name)
	}

	delete(m.plugins, name)

	m.logger.WithField("plugin", name).Info("plugin unregistered")
	return nil
}

// removePluginHooks removes all hooks belonging to a plugin for a given hook name.
// Must be called with the write lock held.
func (m *PluginManager) removePluginHooks(hookName, pluginName string) {
	hooks, exists := m.hooks[hookName]
	if !exists {
		return
	}

	// Filter out hooks from this plugin
	// Since HookFunc is a function type, we can't compare them directly.
	// Instead, we rebuild the hook list by re-registering remaining plugins' hooks.
	filtered := make([]HookFunc, 0, len(hooks))
	for _, p := range m.plugins {
		if p.Name == pluginName {
			continue
		}
		if fn, ok := p.Hooks[hookName]; ok {
			filtered = append(filtered, fn)
		}
	}

	if len(filtered) == 0 {
		delete(m.hooks, hookName)
	} else {
		m.hooks[hookName] = filtered
	}
}

// ListPlugins lists all registered plugins.
func (m *PluginManager) ListPlugins() []*Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]*Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// RegisterHook registers a hook function for a hook name.
func (m *PluginManager) RegisterHook(hookName string, fn HookFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.hooks[hookName] = append(m.hooks[hookName], fn)
	m.logger.WithField("hook", hookName).Debug("hook registered")
}

// ExecuteHooks executes all hooks registered for a hook name.
func (m *PluginManager) ExecuteHooks(ctx context.Context, hookName string, data map[string]interface{}) error {
	m.mu.RLock()
	hooksCopy := make([]HookFunc, len(m.hooks[hookName]))
	copy(hooksCopy, m.hooks[hookName])
	m.mu.RUnlock()

	if len(hooksCopy) == 0 {
		return nil
	}

	m.logger.WithFields(logrus.Fields{
		"hook":       hookName,
		"hook_count": len(hooksCopy),
	}).Debug("executing hooks")

	for _, fn := range hooksCopy {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := fn(ctx, data); err != nil {
			m.logger.WithFields(logrus.Fields{
				"hook":  hookName,
				"error": err,
			}).Warn("hook execution failed")
			// Continue executing other hooks even if one fails
		}
	}

	return nil
}

// Skill represents a reusable skill/template.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

// SkillManager manages skills.
type SkillManager struct {
	skills map[string]*Skill
	logger *logrus.Logger
	mu     sync.RWMutex
}

// NewSkillManager creates a new SkillManager.
func NewSkillManager(logger *logrus.Logger) *SkillManager {
	return &SkillManager{
		skills: make(map[string]*Skill),
		logger: logger,
	}
}

// RegisterSkill registers a skill.
func (m *SkillManager) RegisterSkill(skill *Skill) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.skills[skill.Name] = skill
	m.logger.WithField("skill", skill.Name).Info("skill registered")
}

// GetSkill returns a skill by name.
func (m *SkillManager) GetSkill(name string) (*Skill, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	skill, ok := m.skills[name]
	return skill, ok
}

// ListSkills lists all skills.
func (m *SkillManager) ListSkills() []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	skills := make([]*Skill, 0, len(m.skills))
	for _, s := range m.skills {
		skills = append(skills, s)
	}
	return skills
}

// SearchSkills searches skills by name or description.
func (m *SkillManager) SearchSkills(query string) []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query = strings.ToLower(query)
	var results []*Skill
	for _, s := range m.skills {
		if strings.Contains(strings.ToLower(s.Name), query) ||
			strings.Contains(strings.ToLower(s.Description), query) {
			results = append(results, s)
		}
	}
	return results
}
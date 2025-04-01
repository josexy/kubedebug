package config

import (
	"errors"
	"os"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

const (
	DeploymentType  ResourceType = "deployment"
	StatefulsetType ResourceType = "statefulset"
)

var cfg atomic.Pointer[DebugK8SPodConfig]

type ResourceType string

type DebugK8SPodConfig struct {
	Name           string            `yaml:"name" mapstructure:"name"`
	Namespace      string            `yaml:"namespace" mapstructure:"namespace"`
	Type           ResourceType      `yaml:"type" mapstructure:"type"`
	LabelSelector  map[string]string `yaml:"labelSelector" mapstructure:"labelSelector"`
	FieldSelector  map[string]string `yaml:"fieldSelector" mapstructure:"fieldSelector"`
	ContainerName  string            `yaml:"containerName" mapstructure:"containerName"`
	CommandArgs    []string          `yaml:"commandArgs" mapstructure:"commandArgs"`
	ProjectRootDir string            `yaml:"projectRootDir" mapstructure:"projectRootDir"`
	DebugExePath   string            `yaml:"debugExePath" mapstructure:"debugExePath"`
	DlvExePath     string            `yaml:"dlvExePath" mapstructure:"dlvExePath"`
	NodeHost       string            `yaml:"nodeHost" mapstructure:"nodeHost"`
	NodePort       int               `yaml:"nodePort" mapstructure:"nodePort"`
}

func ReadAndValidateConfigFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	c := new(DebugK8SPodConfig)
	if err = yaml.Unmarshal(data, c); err != nil {
		return err
	}
	if err := validateConfig(c); err != nil {
		return err
	}
	cfg.Store(c)
	return nil
}

func GetConfig() *DebugK8SPodConfig { return cfg.Load() }

func validateConfig(config *DebugK8SPodConfig) error {
	if config.Name == "" {
		return errors.New("name is required")
	}
	if config.Namespace == "" {
		return errors.New("namespace is required")
	}
	if config.ContainerName == "" {
		return errors.New("containerName is required")
	}
	if config.ProjectRootDir == "" {
		return errors.New("projectRootDir is required")
	}
	if len(config.CommandArgs) == 0 {
		return errors.New("commandArgs is required")
	}
	if len(config.FieldSelector) == 0 && len(config.LabelSelector) == 0 {
		return errors.New("labelSelector or fieldSelector is required")
	}
	if config.NodePort <= 30000 || config.NodePort >= 32767 {
		return errors.New("nodePort must be in range 30000-32767")
	}
	if config.DlvExePath == "" {
		return errors.New("dlvExePath is required")
	}
	return nil
}

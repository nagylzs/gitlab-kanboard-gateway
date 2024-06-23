package config

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"regexp"
	"text/template"
	"time"
)

// Command line options
type GatewayOptsType struct {
	ConfigFile  string `short:"c" long:"config" description:"Config file path"`
	Verbose     bool   `short:"v" long:"verbose" description:"Verbose loglevel"`
	Debug       bool   `short:"d" long:"debug" description:"Debug loglevel"`
	ShowVersion bool   `long:"version" description:"Show version information and exit"`
	ShowInfo    bool   `short:"i" long:"info" description:"Show information about the program"`
}

// Initialize with default options
var GatewayOpts GatewayOptsType = GatewayOptsType{
	Verbose: false,
	Debug:   false,
}

type KanboardConfig struct {
	ApiUrl                   string   `yaml:"ApiUrl"`
	Username                 string   `yaml:"Username"`
	Password                 string   `yaml:"Password"`
	TaskRefsStrings          []string `yaml:"TaskRefs"`
	RefStrings               []string `yaml:"Refs"`
	UserId                   int      `yaml:"UserId"`
	MinRefreshIntervalString string   `yaml:"MinRefreshInterval"`
	DefRefreshIntervalString string   `yaml:"DefRefreshInterval"`

	TaskRefPatterns    []*regexp.Regexp
	RefPatterns        []*regexp.Regexp
	MinRefreshInterval time.Duration
	DefRefreshInterval time.Duration
}

type WebhookConfig struct {
	ListenAddress string `yaml:"ListenAddress"`
	SecretToken   string `yaml:"SecretToken"`
}

type Config struct {
	Kanboard              KanboardConfig `yaml:"Kanboard"`
	Webhook               WebhookConfig  `yaml:"Webhook"`
	CommentTemplateString string         `yaml:"CommentTemplate"`
	CommentTemplate       *template.Template
}

func parseDuration(name string, value string) (time.Duration, error) {
	var d time.Duration
	if value == "" {
		return d, fmt.Errorf("%v: should not be empty", name)
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		err = fmt.Errorf("%v: %v", name, err.Error())
	}
	return d, err
}

func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("config file path is required")
	}
	yfile, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Cannot read config file %s: %v", path, err.Error()))
	}
	var config Config
	err = yaml.Unmarshal(yfile, &config)
	if err != nil {
		return nil, fmt.Errorf("Cannot parse YAML file %s: %v", path, err.Error())
	}

	pats, err := compileRegexList(path, "TaskRefs", config.Kanboard.TaskRefsStrings)
	if err != nil {
		return nil, err
	}
	config.Kanboard.TaskRefPatterns = pats

	pats, err = compileRegexList(path, "Refs", config.Kanboard.RefStrings)
	if err != nil {
		return nil, err
	}
	config.Kanboard.RefPatterns = pats

	d, err := parseDuration("MinRefreshInterval", config.Kanboard.MinRefreshIntervalString)
	if err != nil {
		return nil, err
	}
	config.Kanboard.MinRefreshInterval = d

	d, err = parseDuration("DefRefreshInterval", config.Kanboard.DefRefreshIntervalString)
	if err != nil {
		return nil, err
	}
	config.Kanboard.DefRefreshInterval = d

	if config.Kanboard.MinRefreshInterval < time.Second {
		return nil, errors.New("MinRefreshInterval should not be less than 1 second")
	}

	if config.Kanboard.MinRefreshInterval > config.Kanboard.DefRefreshInterval {
		return nil, errors.New("MinRefreshInterval should not be more than DefRefreshInterval")
	}

	if config.CommentTemplateString == "" {
		return nil, errors.New("CommentTemplate should not be empty")
	}

	tmpl, err := template.New("CommentTemplate").Parse(config.CommentTemplateString)
	if err != nil {
		return nil, fmt.Errorf("cannot parse CommentTemplate: %v", err)
	}
	config.CommentTemplate = tmpl

	return &config, nil
}

func compileRegexList(path string, name string, patternStrings []string) ([]*regexp.Regexp, error) {
	pats := make([]*regexp.Regexp, 0)
	for _, r := range patternStrings {
		re, err := regexp.Compile(r)
		if err != nil {
			return nil, fmt.Errorf(
				"cannot compile regexp '%v' in config file %s / %s: %w", r, path, name, err)
		}
		pats = append(pats, re)
	}
	if len(pats) == 0 {
		return nil, fmt.Errorf("at least regexp must be given in config %s / %s", path, name)
	}
	return pats, nil
}

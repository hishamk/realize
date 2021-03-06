package main

import (
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// settings const
const (
	Permission = 0775
	Directory  = ".realize"
	File       = "realize.yaml"
	FileOut    = "outputs.log"
	FileErr    = "errors.log"
	FileLog    = "logs.log"
)

// random string preference
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
	letterBytes   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// Settings defines a group of general settings and options
type Settings struct {
	File      string `yaml:"-" json:"-"`
	Files     `yaml:"files,omitempty" json:"files,omitempty"`
	Server    `yaml:"server,omitempty" json:"server,omitempty"`
	FileLimit int64 `yaml:"flimit,omitempty" json:"flimit,omitempty"`
}

// Server settings, used for the web panel
type Server struct {
	Status bool   `yaml:"status" json:"status"`
	Open   bool   `yaml:"open" json:"open"`
	Host   string `yaml:"host" json:"host"`
	Port   int    `yaml:"port" json:"port"`
}

// Files defines the files generated by realize
type Files struct {
	Outputs Resource `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	Logs    Resource `yaml:"logs,omitempty" json:"log,omitempty"`
	Errors  Resource `yaml:"errors,omitempty" json:"error,omitempty"`
}

// Resource status and file name
type Resource struct {
	Status bool
	Name   string
}

// Rand is used for generate a random string
func random(n int) string {
	src := rand.NewSource(time.Now().UnixNano())
	b := make([]byte, n)
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}
	return string(b)
}

// Wdir return the current working Directory
func (s Settings) wdir() string {
	dir, err := os.Getwd()
	s.validate(err)
	return filepath.Base(dir)
}

// Flimit defines the max number of watched files
func (s *Settings) flimit() error {
	return nil
}

// Delete realize folder
func (s *Settings) delete(d string) error {
	_, err := os.Stat(d)
	if !os.IsNotExist(err) {
		return os.RemoveAll(d)
	}
	return err
}

// Path cleaner
func (s Settings) path(path string) string {
	return strings.Replace(filepath.Clean(path), "\\", "/", -1)
}

// Validate checks a fatal error
func (s Settings) validate(err error) error {
	if err != nil {
		s.fatal(err, "")
	}
	return nil
}

// Read from config file
func (s *Settings) read(out interface{}) error {
	localConfigPath := s.File
	// backward compatibility
	path := filepath.Join(Directory, s.File)
	if _, err := os.Stat(path); err == nil {
		localConfigPath = path
	}
	content, err := s.stream(localConfigPath)
	if err == nil {
		err = yaml.Unmarshal(content, out)
		return err
	}
	return err
}

// Record create and unmarshal the yaml config file
func (s *Settings) record(out interface{}) error {
	y, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	if _, err := os.Stat(Directory); os.IsNotExist(err) {
		if err = os.Mkdir(Directory, Permission); err != nil {
			return s.write(s.File, y)
		}
	}
	return s.write(filepath.Join(Directory, s.File), y)
}

// Stream return a byte stream of a given file
func (s Settings) stream(file string) ([]byte, error) {
	_, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	content, err := ioutil.ReadFile(file)
	s.validate(err)
	return content, err
}

// Fatal prints a fatal error with its additional messages
func (s Settings) fatal(err error, msg ...interface{}) {
	if len(msg) > 0 && err != nil {
		log.Fatalln(red.regular(msg...), err.Error())
	} else if err != nil {
		log.Fatalln(err.Error())
	}
}

// Write a file
func (s Settings) write(name string, data []byte) error {
	err := ioutil.WriteFile(name, data, Permission)
	return s.validate(err)
}

// Name return the project name or the path of the working dir
func (s Settings) name(name string, path string) string {
	if name == "" && path == "" {
		return s.wdir()
	} else if path != "/" {
		return filepath.Base(path)
	}
	return name
}

// Create a new file and return its pointer
func (s Settings) create(path string, name string) *os.File {
	var file string
	if _, err := os.Stat(Directory); err == nil {
		file = filepath.Join(path, Directory, name)
	} else {
		file = filepath.Join(path, name)
	}
	out, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY|os.O_CREATE|os.O_SYNC, Permission)
	s.validate(err)
	return out
}

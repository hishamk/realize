package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// GoCompile is used for compile a project
func (p *Project) goCompile(stop <-chan bool, args []string) (string, error) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	done := make(chan error)
	err := os.Setenv("GOBIN", filepath.Join(getEnvPath("GOPATH"), "bin"))
	if err != nil {
		return "", err
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = p.base
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	// Start command
	cmd.Start()
	go func() { done <- cmd.Wait() }()
	// Wait a result
	select {
	case <-stop:
		// Stop running command
		cmd.Process.Kill()
		return "killed", nil
	case err := <-done:
		// Command completed
		if err != nil {
			return stderr.String(), err
		}
	}
	return "", nil
}

// GoRun  is an implementation of the bin execution
func (p *Project) goRun(stop <-chan bool, runner chan bool) {
	var build *exec.Cmd
	var args []string
	isErrorText := func(string) bool {
		return false
	}
	errRegexp, err := regexp.Compile(p.ErrorOutputPattern)
	if err != nil {
		msg := fmt.Sprintln(p.pname(p.Name, 3), ":", blue.regular(err.Error()))
		out := BufferOut{Time: time.Now(), Text: err.Error(), Type: "Go Run"}
		p.stamp("error", out, msg, "")
	} else {
		isErrorText = func(t string) bool {
			return errRegexp.MatchString(t)
		}
	}
	for _, arg := range p.Args {
		arr := strings.Fields(arg)
		args = append(args, arr...)
	}

	if _, err := os.Stat(filepath.Join(p.base, p.path)); err == nil {
		p.path = filepath.Join(p.base, p.path)
	}
	if _, err := os.Stat(filepath.Join(p.base, p.path+".exe")); err == nil {
		p.path = filepath.Join(p.base, p.path+".exe")
	}

	if _, err := os.Stat(filepath.Join(getEnvPath("GOBIN"), filepath.Base(p.path))); err == nil {
		build = exec.Command(filepath.Join(getEnvPath("GOBIN"), filepath.Base(p.path)), args...)
	} else if _, err := os.Stat(filepath.Join(getEnvPath("GOBIN"), filepath.Base(p.path)) + ".exe"); err == nil {
		build = exec.Command(filepath.Join(getEnvPath("GOBIN"), filepath.Base(p.path))+".exe", args...)
	} else {
		p.Buffer.StdLog = append(p.Buffer.StdLog, BufferOut{Time: time.Now(), Text: "Can't run a not compiled project"})
		p.fatal(err, "Can't run a not compiled project", ":")
	}

	defer func() {
		if err := build.Process.Kill(); err != nil {
			p.Buffer.StdLog = append(p.Buffer.StdLog, BufferOut{Time: time.Now(), Text: "Failed to stop: " + err.Error()})
			p.fatal(err, "Failed to stop", ":")
		}
		msg := fmt.Sprintln(p.pname(p.Name, 2), ":", red.regular("Ended"))
		out := BufferOut{Time: time.Now(), Text: "Ended", Type: "Go Run"}
		p.stamp("log", out, msg, "")
	}()

	stdout, err := build.StdoutPipe()
	stderr, err := build.StderrPipe()
	if err != nil {
		log.Println(red.bold(err.Error()))
		return
	}
	if err := build.Start(); err != nil {
		log.Println(red.bold(err.Error()))
		return
	}
	close(runner)

	execOutput, execError := bufio.NewScanner(stdout), bufio.NewScanner(stderr)
	stopOutput, stopError := make(chan bool, 1), make(chan bool, 1)
	scanner := func(stop chan bool, output *bufio.Scanner, isError bool) {
		for output.Scan() {
			text := output.Text()
			msg := fmt.Sprintln(p.pname(p.Name, 3), ":", blue.regular(text))
			if isError && !isErrorText(text) {
				out := BufferOut{Time: time.Now(), Text: text, Type: "Go Run"}
				p.stamp("error", out, msg, "")
			} else {
				out := BufferOut{Time: time.Now(), Text: text, Type: "Go Run"}
				p.stamp("out", out, msg, "")
			}
		}
		close(stop)
	}
	go scanner(stopOutput, execOutput, false)
	go scanner(stopError, execError, true)
	for {
		select {
		case <-stop:
			return
		case <-stopOutput:
			return
		case <-stopError:
			return
		}
	}
}

// Exec an additional command from a defined path if specified
func (p *Project) command(stop <-chan bool, cmd Command) (string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	done := make(chan error)
	args := strings.Split(strings.Replace(strings.Replace(cmd.Command, "'", "", -1), "\"", "", -1), " ")
	exec := exec.Command(args[0], args[1:]...)
	exec.Dir = p.base
	if cmd.Path != "" {
		if strings.Contains(cmd.Path, p.base) {
			exec.Dir = cmd.Path
		} else {
			exec.Dir = filepath.Join(p.base, cmd.Path)
		}
	}
	exec.Stdout = &stdout
	exec.Stderr = &stderr
	// Start command
	exec.Start()
	go func() { done <- exec.Wait() }()
	// Wait a result
	select {
	case <-stop:
		// Stop running command
		exec.Process.Kill()
		return "", ""
	case err := <-done:
		// Command completed
		if err != nil {
			return stderr.String(), stdout.String()
		}
	}
	return "", stdout.String()
}

// GoTool is used for run go tools methods such as fmt, test, generate and so on
func (p *Project) goTool(wg *sync.WaitGroup, stop <-chan bool, result chan<- tool, path string, tool tool) {
	defer wg.Done()
	if tool.status {
		if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "") {
			if strings.HasSuffix(path, ".go") {
				tool.options = append(tool.options, path)
				path = p.base
			}
			if s := ext(path); s == "" || s == "go" {
				if tool.dir {
					path = filepath.Dir(path)
				}
				var out, stderr bytes.Buffer
				done := make(chan error)
				cmd := exec.Command(tool.cmd, tool.options...)
				cmd.Dir = path
				cmd.Stdout = &out
				cmd.Stderr = &stderr
				// Start command
				cmd.Start()
				go func() { done <- cmd.Wait() }()
				// Wait a result
				select {
				case <-stop:
					// Stop running command
					cmd.Process.Kill()
					break
				case err := <-done:
					// Command completed
					if err != nil {
						tool.err = stderr.String() + out.String()
						// send command result
						result <- tool
					}
					break
				}

			}
		}
	}
}
package cmd

import (
	"flag"
	"io/ioutil"

	"github.com/mattn/go-shellwords"
)

func ParseFlagFile(path string) error {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	p := shellwords.NewParser()
	p.ParseEnv = true
	args, err := p.Parse(string(file))
	if err != nil {
		return err
	}

	return flag.CommandLine.Parse(args)
}

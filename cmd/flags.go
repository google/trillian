package cmd

import (
	"flag"
	"io/ioutil"
)

// ParseFlagFile parses a set of flags from a file at the provided
// path. Re-calls flag.Parse() after parsing the flags in the file
// so that flags provided on the command line take precedence over
// flags provided in the file.
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

	err = flag.CommandLine.Parse(args)
	if err != nil {
		return err
	}

	// Call flag.Parse() again so that command line flags
	// can override flags provided in the provided flag file.
	flag.Parse()
	return nil
}

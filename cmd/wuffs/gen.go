// Copyright 2017 The Wuffs Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

func doGen(wuffsRoot string, args []string) error    { return doGenGenlib(wuffsRoot, args, false) }
func doGenlib(wuffsRoot string, args []string) error { return doGenGenlib(wuffsRoot, args, true) }

func doGenGenlib(wuffsRoot string, args []string, genlib bool) error {
	flags := flag.NewFlagSet("gen", flag.ExitOnError)
	langsFlag := flags.String("langs", langsDefault, langsUsage)
	if err := flags.Parse(args); err != nil {
		return err
	}
	langs, err := parseLangs(*langsFlag)
	if err != nil {
		return err
	}
	args = flags.Args()
	if len(args) == 0 {
		args = []string{"std/..."}
	}

	h := genHelper{
		wuffsRoot: wuffsRoot,
		langs:     langs,
	}

	for _, arg := range args {
		recursive := strings.HasSuffix(arg, "/...")
		if recursive {
			arg = arg[:len(arg)-4]
		}
		if err := h.gen(arg, recursive); err != nil {
			return err
		}
	}

	if genlib {
		return h.genlibAffected()
	}
	return nil
}

type genHelper struct {
	wuffsRoot string
	langs     []string
	affected  []string
}

func (h *genHelper) gen(dirname string, recursive bool) error {
	filenames, dirnames, err := listDir(h.wuffsRoot, dirname, recursive)
	if err != nil {
		return err
	}
	if len(filenames) > 0 {
		if err := h.genDir(dirname, filenames); err != nil {
			return err
		}
		h.affected = append(h.affected, dirname)
	}
	if len(dirnames) > 0 {
		for _, d := range dirnames {
			if err := h.gen(dirname+"/"+d, recursive); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *genHelper) genDir(dirname string, filenames []string) error {
	// TODO: skip the generation if the output file already exists and its
	// mtime is newer than all inputs and the wuffs-gen-foo command.

	packageName := path.Base(dirname)
	if !validName(packageName) {
		return fmt.Errorf(`invalid package %q, not in [a-z0-9]+`, packageName)
	}
	cmdArgs := []string{"gen", "-package_name", packageName}
	for _, filename := range filenames {
		cmdArgs = append(cmdArgs,
			filepath.Join(h.wuffsRoot, filepath.FromSlash(dirname), filename))
	}

	for _, lang := range h.langs {
		command := "wuffs-" + lang
		stdout := &bytes.Buffer{}
		cmd := exec.Command(command, cmdArgs...)
		cmd.Stdin = nil
		cmd.Stdout = stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			// No-op.
		} else if _, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s: failed", command)
		} else {
			return err
		}
		out := stdout.Bytes()
		if err := h.genFile(dirname, lang, out); err != nil {
			return err
		}

		// Special-case the "c" generator to also write a .h file.
		if lang != "c" {
			continue
		}
		if i := bytes.Index(out, cHeaderEndsHere); i < 0 {
			return fmt.Errorf("%s: output did not contain %q", command, cHeaderEndsHere)
		} else {
			out = out[:i]
		}
		if err := h.genFile(dirname, "h", out); err != nil {
			return err
		}
	}
	return nil
}

var cHeaderEndsHere = []byte("\n// C HEADER ENDS HERE.\n\n")

func (h *genHelper) genFile(dirname string, lang string, out []byte) error {
	outFilename := filepath.Join(h.wuffsRoot, "gen", lang, filepath.FromSlash(dirname)+"."+lang)
	if existing, err := ioutil.ReadFile(outFilename); err == nil && bytes.Equal(existing, out) {
		fmt.Println("gen unchanged: ", outFilename)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(outFilename), 0755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(outFilename, out, 0644); err != nil {
		return err
	}
	fmt.Println("gen wrote:     ", outFilename)
	return nil
}

func (h *genHelper) genlibAffected() error {
	for _, lang := range h.langs {
		command := "wuffs-" + lang
		args := []string{"genlib"}
		args = append(args, "-dstdir", filepath.Join(h.wuffsRoot, "gen", "lib", lang))
		args = append(args, "-srcdir", filepath.Join(h.wuffsRoot, "gen", lang))
		args = append(args, h.affected...)
		cmd := exec.Command(command, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
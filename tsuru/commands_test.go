// Copyright 2015 yati authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"net/http"

	"github.com/tsuru/tsuru/cmd"
	"gopkg.in/check.v1"
)

func (s *S) TestInstallInfo(c *check.C) {
	c.Assert((&install{}).Info(), check.NotNil)
}

func (s *S) TestInstall(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	client := cmd.NewClient(&http.Client{}, nil, manager)
	command := install{}
	err := command.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "")
}

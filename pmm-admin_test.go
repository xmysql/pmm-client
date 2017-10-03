/*
	Copyright (c) 2016, Percona LLC and/or its affiliates. All rights reserved.

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/percona/pmm-client/pmm"
	"github.com/percona/pmm-client/test/fakeapi"
	"github.com/percona/pmm/proto"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

type pmmAdminData struct {
	bin     string
	rootDir string
}

func TestPmmAdmin(t *testing.T) {
	var err error

	// We can't/shouldn't use /usr/local/percona/ (the default basedir), so use
	// a tmpdir instead with roughly the same structure.
	rootDir, err := ioutil.TempDir("/tmp", "pmm-client-test-rootdir-")
	assert.Nil(t, err)
	defer func() {
		err := os.RemoveAll(rootDir)
		assert.Nil(t, err)
	}()

	binDir, err := ioutil.TempDir("/tmp", "pmm-client-test-bindir-")
	assert.Nil(t, err)
	defer func() {
		err := os.RemoveAll(binDir)
		assert.Nil(t, err)
	}()

	bin := binDir + "/pmm-admin"
	xVariables := map[string]string{
		"github.com/percona/pmm-client/pmm.Version": "gotest",
		"github.com/percona/pmm-client/pmm.RootDir": rootDir,
	}
	var ldflags []string
	for x, value := range xVariables {
		ldflags = append(ldflags, fmt.Sprintf("-X %s=%s", x, value))
	}
	cmd := exec.Command(
		"go",
		"build",
		"-o",
		bin,
		"-ldflags",
		strings.Join(ldflags, " "),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	assert.Nil(t, err, "Failed to build: %s", err)

	data := pmmAdminData{
		bin:     bin,
		rootDir: rootDir,
	}
	tests := []func(*testing.T, pmmAdminData){
		testVersion,
		testConfig,
		testConfigVerbose,
		testConfigVerboseServerNotAvailable,
		testStartStopRestart,
		testStartStopRestartAllWithNoServices,
		testStartStopRestartAllWithServices,
		testStartStopRestartNoServiceFound,
		testCheckNetwork,
		testAddMongoDB,
		testAddMongoDBQueries,
		testAddLinuxMetricsWithAdditionalArgsOk,
		testAddLinuxMetricsWithAdditionalArgsFail,
	}
	t.Run("pmm-admin", func(t *testing.T) {
		for _, f := range tests {
			f := f // capture range variable
			fName := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
			t.Run(fName, func(t *testing.T) {
				// t.Parallel()
				f(t, data)
			})
		}
	})

}

func testVersion(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	cmd := exec.Command(
		data.bin,
		"--version",
	)
	output, err := cmd.CombinedOutput()
	assert.Nil(t, err)

	// sanity check that version number was changed with ldflag for this test build
	assert.Equal(t, "EXPERIMENTAL", pmm.Version)
	expected := `gotest`

	assertRegexpLines(t, expected, string(output))
}

func testConfig(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)

	// Create fake api server
	fapi := fakeapi.New()
	u, _ := url.Parse(fapi.URL())
	clientAddress, _, _ := net.SplitHostPort(u.Host)
	clientName, _ := os.Hostname()
	fapi.AppendRoot()
	fapi.AppendConsulV1StatusLeader(clientAddress)
	node := api.CatalogNode{
		Node: &api.Node{},
	}
	fapi.AppendConsulV1CatalogNode(clientName, node)

	cmd := exec.Command(
		data.bin,
		"config",
		"--server",
		u.Host,
	)

	output, err := cmd.CombinedOutput()
	assert.Nil(t, err)

	expected := `OK, PMM server is alive.

` + fmt.Sprintf("%-15s | %s ", "PMM Server", u.Host) + `
` + fmt.Sprintf("%-15s | %s", "Client Name", clientName) + `
` + fmt.Sprintf("%-15s | %s ", "Client Address", clientAddress) + `
`
	assertRegexpLines(t, expected, string(output))
}

func testConfigVerbose(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)

	// Create fake api server
	fapi := fakeapi.New()
	u, _ := url.Parse(fapi.URL())
	clientAddress, _, _ := net.SplitHostPort(u.Host)
	clientName, _ := os.Hostname()
	fapi.AppendRoot()
	fapi.AppendConsulV1StatusLeader(clientAddress)
	node := api.CatalogNode{
		Node: &api.Node{},
	}
	fapi.AppendConsulV1CatalogNode(clientName, node)

	cmd := exec.Command(
		data.bin,
		"config",
		"--verbose",
		"--server",
		u.Host,
	)

	output, err := cmd.CombinedOutput()
	assert.Nil(t, err)

	// with --verbose flag we should have bunch of http requests to server
	expected := `.+ request:
> GET / HTTP/1.1
> Host: ` + u.Host + `
> User-Agent: Go-http-client/1.1
> Accept-Encoding: gzip
>\s*
>\s*
.+ response:
< HTTP/1.1 200 OK
< Content-Type: text/plain; charset=utf-8
< Date: .*
< Content-Length: 0
<\s*
<\s*
.+ request:
> GET /v1/status/leader HTTP/1.1
> Host: ` + u.Host + `
> User-Agent: Go-http-client/1.1
> Accept-Encoding: gzip
>\s*
>\s*
.+ response:
< HTTP/1.1 200 OK
< Content-Length: 16
< Content-Type: text/plain; charset=utf-8
< Date: .*
< X-Remote-Ip: 127.0.0.1
< X-Server-Time: .*
<\s*
< "127.0.0.1:8300"
.+ request:
> GET /v1/catalog/node/`+clientName+` HTTP/1.1
> Host: ` + u.Host + `
> User-Agent: Go-http-client/1.1
> Accept-Encoding: gzip
>\s*
>\s*
.+ response:
< HTTP/1.1 200 OK
< Content-Length: 140
< Content-Type: text/plain; charset=utf-8
< Date: .*
<\s*
< {"Node":{"ID":"","Node":"","Address":"","Datacenter":"","TaggedAddresses":null,"Meta":null,"CreateIndex":0,"ModifyIndex":0},"Services":null}
.+ request:
> GET /v1/status/leader HTTP/1.1
> Host: ` + u.Host + `
> User-Agent: Go-http-client/1.1
> Accept-Encoding: gzip
>\s*
>\s*
.+ response:
< HTTP/1.1 200 OK
< Content-Length: 16
< Content-Type: text/plain; charset=utf-8
< Date: .*
< X-Remote-Ip: 127.0.0.1
< X-Server-Time: .*
<\s*
< "127.0.0.1:8300"
OK, PMM server is alive.

PMM Server      | ` + u.Host + `
Client Name     | ` + clientName + `
Client Address  | ` + clientAddress + `
`

	assertRegexpLines(t, expected, string(output))
}

func testConfigVerboseServerNotAvailable(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)

	cmd := exec.Command(
		data.bin,
		"config",
		"--verbose",
		"--server",
		"xyz",
	)

	output, err := cmd.CombinedOutput()
	assert.Error(t, err)

	// with --verbose flag we should have bunch of http requests to server
	// however api is unavailable, so `--verbose` prints only request...
	expected := `.* request:
> GET / HTTP/1.1
> Host: xyz
> User-Agent: Go-http-client/1.1
> Accept-Encoding: gzip
>\s*
>\s*
Unable to connect to PMM server by address: xyz
Get http://xyz: dial tcp: lookup xyz.*: no such host

* Check if the configured address is correct.
* If server is running on non-default port, ensure it was specified along with the address.
* If server is enabled for SSL or self-signed SSL, enable the corresponding option.
* You may also check the firewall settings.
`
	assertRegexpLines(t, expected, string(output))
}

func testStartStopRestartAllWithNoServices(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)
	os.Create(data.rootDir + pmm.PMMBaseDir + "/node_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mysqld_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mongodb_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/proxysql_exporter")

	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/bin", 0777)
	os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent")
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/config", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/instance", 0777)

	f, _ := os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent-installer")
	f.WriteString("#!/bin/sh\n")
	f.WriteString("echo 'it works'")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	f, _ = os.Create(data.rootDir + pmm.AgentBaseDir + "/config/agent.conf")
	f.WriteString(`{"UUID":"42","ApiHostname":"somehostname","ApiPath":"/qan-api","ServerUser":"pmm"}`)
	f.WriteString("\n")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	pmmConfig := pmm.Config{
		ServerAddress: "just",
		ClientName:    "non",
		ClientAddress: "empty",
		BindAddress:   "data",
	}
	bytes, _ := yaml.Marshal(pmmConfig)
	ioutil.WriteFile(data.rootDir+pmm.PMMBaseDir+"/pmm.yml", bytes, 0600)

	services := []string{
		"start",
		"stop",
		"restart",
	}
	t.Run("service", func(t *testing.T) {
		for _, service := range services {
			service := service // capture range variable
			t.Run(service, func(t *testing.T) {
				t.Parallel()
				cmd := exec.Command(
					data.bin,
					service,
					"--all",
				)

				output, err := cmd.CombinedOutput()
				assert.Nil(t, err)
				expected := `OK, no services found.`
				assertRegexpLines(t, expected, string(output))
			})
		}
	})
}

func testStartStopRestart(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	svcName := "mysql:queries"

	// Create fake api server
	fapi := fakeapi.New()
	u, _ := url.Parse(fapi.URL())
	serverAddress, _, _ := net.SplitHostPort(u.Host)
	clientName := "test-client-name"
	fapi.AppendRoot()
	fapi.AppendConsulV1StatusLeader(serverAddress)
	node := api.CatalogNode{
		Node: &api.Node{},
		Services: map[string]*api.AgentService{
			"a": {
				ID:      "id",
				Service: svcName,
				Port:    0,
				Tags: []string{
					fmt.Sprintf("alias_%s", clientName),
				},
			},
		},
	}
	fapi.AppendConsulV1CatalogNode(clientName, node)

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)
	os.Create(data.rootDir + pmm.PMMBaseDir + "/node_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mysqld_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mongodb_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/proxysql_exporter")

	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/bin", 0777)
	os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent")
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/config", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/instance", 0777)

	f, _ := os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent-installer")
	f.WriteString("#!/bin/sh\n")
	f.WriteString("echo 'it works'")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	f, _ = os.Create(data.rootDir + pmm.AgentBaseDir + "/config/agent.conf")
	f.WriteString(`{"UUID":"42","ApiHostname":"somehostname","ApiPath":"/qan-api","ServerUser":"pmm"}`)
	f.WriteString("\n")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	pmmConfig := pmm.Config{
		ServerAddress: fmt.Sprintf("%s:%s", fapi.Host(), fapi.Port()),
		ClientName:    clientName,
		ClientAddress: "empty",
		BindAddress:   "data",
	}
	bytes, _ := yaml.Marshal(pmmConfig)
	ioutil.WriteFile(data.rootDir+pmm.PMMBaseDir+"/pmm.yml", bytes, 0600)

	// create fake system service
	{
		dir, extension := pmm.GetServiceDirAndExtension()
		os.MkdirAll(data.rootDir+dir, 0777)
		name := fmt.Sprintf("pmm-mysql-queries-0%s", extension)
		os.Create(data.rootDir + dir + "/" + name)
	}

	t.Run("start", func(t *testing.T) {
		cmd := exec.Command(
			data.bin,
			"start",
			svcName,
		)

		output, err := cmd.CombinedOutput()
		assert.Nil(t, err)
		expected := fmt.Sprintf("OK, service %s already %s for %s.", svcName, "started", clientName)
		assertRegexpLines(t, expected, string(output))
	})

	t.Run("stop", func(t *testing.T) {
		cmd := exec.Command(
			data.bin,
			"stop",
			svcName,
		)

		output, err := cmd.CombinedOutput()
		assert.Nil(t, err)
		expected := fmt.Sprintf("OK, %s %s service for %s.", "stopped", svcName, clientName)
		assertRegexpLines(t, expected, string(output))
	})

	t.Run("restart", func(t *testing.T) {
		cmd := exec.Command(
			data.bin,
			"restart",
			svcName,
		)

		output, err := cmd.CombinedOutput()
		assert.Nil(t, err)
		expected := fmt.Sprintf("OK, %s %s service for %s.", "restarted", svcName, clientName)
		assertRegexpLines(t, expected, string(output))
	})
}

func testStartStopRestartAllWithServices(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)
	os.Create(data.rootDir + pmm.PMMBaseDir + "/node_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mysqld_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mongodb_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/proxysql_exporter")

	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/bin", 0777)
	os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent")
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/config", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/instance", 0777)

	f, _ := os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent-installer")
	f.WriteString("#!/bin/sh\n")
	f.WriteString("echo 'it works'")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	f, _ = os.Create(data.rootDir + pmm.AgentBaseDir + "/config/agent.conf")
	f.WriteString(`{"UUID":"42","ApiHostname":"somehostname","ApiPath":"/qan-api","ServerUser":"pmm"}`)
	f.WriteString("\n")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	pmmConfig := pmm.Config{
		ServerAddress: "just",
		ClientName:    "non",
		ClientAddress: "empty",
		BindAddress:   "data",
	}
	bytes, _ := yaml.Marshal(pmmConfig)
	ioutil.WriteFile(data.rootDir+pmm.PMMBaseDir+"/pmm.yml", bytes, 0600)

	// create fake system services
	numOfServices := 3
	{
		dir, extension := pmm.GetServiceDirAndExtension()
		os.MkdirAll(data.rootDir+dir, 0777)
		for i := 0; i < numOfServices; i++ {
			name := fmt.Sprintf("pmm-service-%d%s", i, extension)
			os.Create(data.rootDir + dir + "/" + name)
		}
	}

	t.Run("start", func(t *testing.T) {
		cmd := exec.Command(
			data.bin,
			"start",
			"--all",
		)

		output, err := cmd.CombinedOutput()
		assert.Nil(t, err)
		expected := `OK, all services already started. Run 'pmm-admin list' to see monitoring services.
Unable to connect to PMM server by address: just
Get http://just: dial tcp: lookup just.*: no such host

* Check if the configured address is correct.
* If server is running on non-default port, ensure it was specified along with the address.
* If server is enabled for SSL or self-signed SSL, enable the corresponding option.
* You may also check the firewall settings.
`
		assertRegexpLines(t, expected, string(output))
	})

	t.Run("stop", func(t *testing.T) {
		cmd := exec.Command(
			data.bin,
			"stop",
			"--all",
		)

		output, err := cmd.CombinedOutput()
		assert.Nil(t, err)
		expected := fmt.Sprintf("OK, %s %d services.\n", "stopped", numOfServices)
		assertRegexpLines(t, expected, string(output))
	})

	t.Run("restart", func(t *testing.T) {
		cmd := exec.Command(
			data.bin,
			"restart",
			"--all",
		)

		output, err := cmd.CombinedOutput()
		assert.Nil(t, err)
		expected := `OK, restarted ` + fmt.Sprintf("%d", numOfServices) + ` services.
Unable to connect to PMM server by address: just
Get http://just: dial tcp: lookup just.*: no such host

* Check if the configured address is correct.
* If server is running on non-default port, ensure it was specified along with the address.
* If server is enabled for SSL or self-signed SSL, enable the corresponding option.
* You may also check the firewall settings.
`
		assertRegexpLines(t, expected, string(output))
	})
}

func testStartStopRestartNoServiceFound(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	// Create fake api server
	fapi := fakeapi.New()
	fapi.AppendRoot()
	fapi.AppendConsulV1StatusLeader(fapi.Host())
	clientName, _ := os.Hostname()
	node := api.CatalogNode{
		Node: &api.Node{},
	}
	fapi.AppendConsulV1CatalogNode(clientName, node)

	// Create fake filesystem
	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)
	os.Create(data.rootDir + pmm.PMMBaseDir + "/node_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mysqld_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mongodb_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/proxysql_exporter")

	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/bin", 0777)
	os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent")
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/config", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/instance", 0777)

	f, _ := os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent-installer")
	f.WriteString("#!/bin/sh\n")
	f.WriteString("echo 'it works'")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	f, _ = os.Create(data.rootDir + pmm.AgentBaseDir + "/config/agent.conf")
	f.WriteString(`{"UUID":"42","ApiHostname":"somehostname","ApiPath":"/qan-api","ServerUser":"pmm"}`)
	f.WriteString("\n")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	pmmConfig := pmm.Config{
		ServerAddress: fmt.Sprintf("%s:%s", fapi.Host(), fapi.Port()),
		ClientName:    clientName,
		ClientAddress: "localhost",
		BindAddress:   "localhost",
	}
	bytes, _ := yaml.Marshal(pmmConfig)
	ioutil.WriteFile(data.rootDir+pmm.PMMBaseDir+"/pmm.yml", bytes, 0600)
	svcName := "mysql:queries"

	t.Run("start", func(t *testing.T) {
		cmd := exec.Command(
			data.bin,
			"start",
			svcName,
		)

		output, err := cmd.CombinedOutput()
		assert.Error(t, err)
		expected := fmt.Sprintf("Error %s %s service for %s: no service found.\n", "starting", svcName, clientName)
		assertRegexpLines(t, expected, string(output))
	})

	t.Run("stop", func(t *testing.T) {
		cmd := exec.Command(
			data.bin,
			"stop",
			svcName,
		)

		output, err := cmd.CombinedOutput()
		assert.Error(t, err)
		expected := fmt.Sprintf("Error %s %s service for %s: no service found.\n", "stopping", svcName, clientName)
		assertRegexpLines(t, expected, string(output))
	})

	t.Run("restart", func(t *testing.T) {
		cmd := exec.Command(
			data.bin,
			"restart",
			svcName,
		)

		output, err := cmd.CombinedOutput()
		assert.Error(t, err)
		expected := fmt.Sprintf("Error %s %s service for %s: no service found.\n", "restarting", svcName, clientName)
		assertRegexpLines(t, expected, string(output))
	})
}

func testCheckNetwork(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	// Create fake api server
	fapi := fakeapi.New()
	u, _ := url.Parse(fapi.URL())
	fapi.AppendRoot()
	fapi.AppendPrometheusAPIV1Query()
	fapi.AppendQanAPIPing()
	fapi.AppendConsulV1StatusLeader(fapi.Host())
	clientName, _ := os.Hostname()
	node := api.CatalogNode{
		Node: &api.Node{},
	}
	fapi.AppendConsulV1CatalogNode(clientName, node)

	// Create fake filesystem
	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)
	os.Create(data.rootDir + pmm.PMMBaseDir + "/node_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mysqld_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mongodb_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/proxysql_exporter")

	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/bin", 0777)
	os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent")
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/config", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/instance", 0777)

	f, _ := os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent-installer")
	f.WriteString("#!/bin/sh\n")
	f.WriteString("echo 'it works'")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	f, _ = os.Create(data.rootDir + pmm.AgentBaseDir + "/config/agent.conf")
	f.WriteString(`{"UUID":"42","ApiHostname":"somehostname","ApiPath":"/qan-api","ServerUser":"pmm"}`)
	f.WriteString("\n")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	pmmConfig := pmm.Config{
		ServerAddress: fmt.Sprintf("%s:%s", fapi.Host(), fapi.Port()),
		ClientName:    clientName,
		ClientAddress: "localhost",
		BindAddress:   "localhost",
	}
	bytes, _ := yaml.Marshal(pmmConfig)
	ioutil.WriteFile(data.rootDir+pmm.PMMBaseDir+"/pmm.yml", bytes, 0600)

	// Test the command
	{
		cmd := exec.Command(
			data.bin,
			"check-network",
		)

		output, err := cmd.CombinedOutput()
		assert.Nil(t, err)
		expected := `PMM Network Status

Server Address | ` + u.Host + `
Client Address | localhost

* System Time
NTP Server (0.pool.ntp.org)         | .*
PMM Server                          | .*
PMM Client                          | .*
PMM Server Time Drift               | OK
PMM Client Time Drift               | OK
PMM Client to PMM Server Time Drift | OK

* Connection: Client --> Server
-------------------- -------\s*
SERVER SERVICE       STATUS \s*
-------------------- -------\s*
Consul API           OK     \s*
Prometheus API       OK     \s*
Query Analytics API  OK     \s*

Connection duration | .*
Request duration    | .*
Full round trip     | .*


* Connection: Client <-- Server
No metric endpoints registered.

`
		assertRegexpLines(t, expected, string(output))
	}
}

func testAddLinuxMetricsWithAdditionalArgsOk(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/bin", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/config", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/instance", 0777)
	os.Create(data.rootDir + pmm.PMMBaseDir + "/node_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mysqld_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mongodb_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/proxysql_exporter")
	os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent")

	f, _ := os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent-installer")
	f.WriteString("#!/bin/sh\n")
	f.WriteString("echo 'it works'")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	f, _ = os.Create(data.rootDir + pmm.AgentBaseDir + "/config/agent.conf")
	f.WriteString(`{"UUID":"42","ApiHostname":"somehostname","ApiPath":"/qan-api","ServerUser":"pmm"}`)
	f.WriteString("\n")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)
	{
		// Create fake api server
		fapi := fakeapi.New()
		fapi.AppendRoot()
		fapi.AppendConsulV1StatusLeader(fapi.Host())
		clientName, _ := os.Hostname()
		node := api.CatalogNode{
			Node: &api.Node{},
		}
		fapi.AppendConsulV1CatalogNode(clientName, node)
		fapi.AppendConsulV1CatalogService()
		fapi.AppendConsulV1CatalogRegister()

		// Configure pmm
		cmd := exec.Command(
			data.bin,
			"config",
			"--server",
			fmt.Sprintf("%s:%s", fapi.Host(), fapi.Port()),
		)
		output, err := cmd.CombinedOutput()
		assert.Nil(t, err, string(output))
	}

	cmd := exec.Command(
		data.bin,
		"add",
		"linux:metrics",
		"host1",
		"--",
		"--some-additional-params",
		"--for-exporter",
	)

	output, err := cmd.CombinedOutput()
	assert.Nil(t, err)
	expected := `OK, now monitoring this system.`
	assertRegexpLines(t, expected, string(output))
}

func testAddLinuxMetricsWithAdditionalArgsFail(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/bin", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/config", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/instance", 0777)
	os.Create(data.rootDir + pmm.PMMBaseDir + "/node_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mysqld_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mongodb_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/proxysql_exporter")
	os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent")

	f, _ := os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent-installer")
	f.WriteString("#!/bin/sh\n")
	f.WriteString("echo 'it works'")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	f, _ = os.Create(data.rootDir + pmm.AgentBaseDir + "/config/agent.conf")
	f.WriteString(`{"UUID":"42","ApiHostname":"somehostname","ApiPath":"/qan-api","ServerUser":"pmm"}`)
	f.WriteString("\n")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)
	{
		// Create fake api server
		fapi := fakeapi.New()
		fapi.AppendRoot()
		fapi.AppendConsulV1StatusLeader(fapi.Host())
		clientName, _ := os.Hostname()
		node := api.CatalogNode{
			Node: &api.Node{},
		}
		fapi.AppendConsulV1CatalogNode(clientName, node)
		fapi.AppendConsulV1CatalogService()
		fapi.AppendConsulV1CatalogRegister()

		// Configure pmm
		cmd := exec.Command(
			data.bin,
			"config",
			"--server",
			fmt.Sprintf("%s:%s", fapi.Host(), fapi.Port()),
		)
		output, err := cmd.CombinedOutput()
		assert.Nil(t, err, string(output))
	}

	cmd := exec.Command(
		data.bin,
		"add",
		"linux:metrics",
		"host1",
		"too-many-params",
		"--",
		"--some-additional-params",
		"--for-exporter",
	)

	output, err := cmd.CombinedOutput()
	assert.Error(t, err)
	expected := `Too many parameters. Only service name is allowed but got: host1, too-many-params.`
	assertRegexpLines(t, expected, string(output))
}

func testAddMongoDB(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/bin", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/config", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/instance", 0777)
	os.Create(data.rootDir + pmm.PMMBaseDir + "/node_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mysqld_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mongodb_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/proxysql_exporter")
	os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent")

	f, _ := os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent-installer")
	f.WriteString("#!/bin/sh\n")
	f.WriteString("echo 'it works'")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	f, _ = os.Create(data.rootDir + pmm.AgentBaseDir + "/config/agent.conf")
	f.WriteString(`{"UUID":"42","ApiHostname":"somehostname","ApiPath":"/qan-api","ServerUser":"pmm"}`)
	f.WriteString("\n")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)
	{
		// Create fake api server
		fapi := fakeapi.New()
		fapi.AppendRoot()
		fapi.AppendConsulV1StatusLeader(fapi.Host())
		clientName, _ := os.Hostname()
		node := api.CatalogNode{
			Node: &api.Node{},
		}
		fapi.AppendConsulV1CatalogNode(clientName, node)
		fapi.AppendConsulV1CatalogService()
		fapi.AppendConsulV1CatalogRegister()
		mongodbInstance := &proto.Instance{
			Subsystem: "mongodb",
			UUID:      "13",
		}
		agentInstance := &proto.Instance{
			Subsystem: "agent",
			UUID:      "42",
		}
		fapi.AppendQanAPIInstancesId(agentInstance.UUID, agentInstance)
		fapi.AppendQanAPIAgents(agentInstance.UUID)
		fapi.AppendQanAPIInstances([]*proto.Instance{
			mongodbInstance,
		})

		// Configure pmm
		cmd := exec.Command(
			data.bin,
			"config",
			"--server",
			fmt.Sprintf("%s:%s", fapi.Host(), fapi.Port()),
		)
		output, err := cmd.CombinedOutput()
		assert.Nil(t, err, string(output))
	}

	cmd := exec.Command(
		data.bin,
		"add",
		"mongodb",
	)

	output, err := cmd.CombinedOutput()
	assert.Nil(t, err)
	expected := `\[linux:metrics\]   OK, now monitoring this system.
\[mongodb:metrics\] OK, now monitoring MongoDB metrics using URI localhost:27017
\[mongodb:queries\] OK, now monitoring MongoDB queries using URI localhost:27017
\[mongodb:queries\] It is required for correct operation that profiling of monitored MongoDB databases be enabled.
\[mongodb:queries\] Note that profiling is not enabled by default because it may reduce the performance of your MongoDB server.
\[mongodb:queries\] For more information read PMM documentation \(https://www.percona.com/doc/percona-monitoring-and-management/conf-mongodb.html\).
`
	assertRegexpLines(t, expected, string(output))
}

func testAddMongoDBQueries(t *testing.T, data pmmAdminData) {
	defer func() {
		err := os.RemoveAll(data.rootDir)
		assert.Nil(t, err)
	}()

	os.MkdirAll(data.rootDir+pmm.PMMBaseDir, 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/bin", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/config", 0777)
	os.MkdirAll(data.rootDir+pmm.AgentBaseDir+"/instance", 0777)
	os.Create(data.rootDir + pmm.PMMBaseDir + "/node_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mysqld_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/mongodb_exporter")
	os.Create(data.rootDir + pmm.PMMBaseDir + "/proxysql_exporter")
	os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent")

	f, _ := os.Create(data.rootDir + pmm.AgentBaseDir + "/bin/percona-qan-agent-installer")
	f.WriteString("#!/bin/sh\n")
	f.WriteString("echo 'it works'")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)

	f, _ = os.Create(data.rootDir + pmm.AgentBaseDir + "/config/agent.conf")
	f.WriteString(`{"UUID":"42","ApiHostname":"somehostname","ApiPath":"/qan-api","ServerUser":"pmm"}`)
	f.WriteString("\n")
	f.Close()
	os.Chmod(data.rootDir+pmm.AgentBaseDir+"/bin/percona-qan-agent-installer", 0777)
	{
		// Create fake api server
		fapi := fakeapi.New()
		fapi.AppendRoot()
		fapi.AppendConsulV1StatusLeader(fapi.Host())
		clientName, _ := os.Hostname()
		node := api.CatalogNode{
			Node: &api.Node{},
		}
		fapi.AppendConsulV1CatalogNode(clientName, node)
		fapi.AppendConsulV1CatalogService()
		fapi.AppendConsulV1CatalogRegister()
		mongodbInstance := &proto.Instance{
			Subsystem: "mongodb",
			UUID:      "13",
		}
		agentInstance := &proto.Instance{
			Subsystem: "agent",
			UUID:      "42",
		}
		fapi.AppendQanAPIInstancesId(agentInstance.UUID, agentInstance)
		fapi.AppendQanAPIAgents(agentInstance.UUID)
		fapi.AppendQanAPIInstances([]*proto.Instance{
			mongodbInstance,
		})

		// Configure pmm
		cmd := exec.Command(
			data.bin,
			"config",
			"--server",
			fmt.Sprintf("%s:%s", fapi.Host(), fapi.Port()),
		)
		output, err := cmd.CombinedOutput()
		assert.Nil(t, err, string(output))
	}

	cmd := exec.Command(
		data.bin,
		"add",
		"mongodb:queries",
	)

	output, err := cmd.CombinedOutput()
	assert.Nil(t, err)
	expected := `OK, now monitoring MongoDB queries using URI localhost:27017
It is required for correct operation that profiling of monitored MongoDB databases be enabled.
Note that profiling is not enabled by default because it may reduce the performance of your MongoDB server.
For more information read PMM documentation \(https://www.percona.com/doc/percona-monitoring-and-management/conf-mongodb.html\).
`
	assertRegexpLines(t, expected, string(output))
}

// assertRegexpLines matches regexp line by line to corresponding line of text
func assertRegexpLines(t *testing.T, rx string, str string, msgAndArgs ...interface{}) bool {
	expectedScanner := bufio.NewScanner(strings.NewReader(rx))
	defer func() {
		if err := expectedScanner.Err(); err != nil {
			t.Fatal(err)
		}
	}()

	actualScanner := bufio.NewScanner(strings.NewReader(str))
	defer func() {
		if err := actualScanner.Err(); err != nil {
			t.Fatal(err)
		}
	}()

	ok := true
	for {
		asOk := actualScanner.Scan()
		esOk := expectedScanner.Scan()

		switch {
		case asOk && esOk:
			ok = ok && assert.Regexp(t, "^"+expectedScanner.Text()+"$", actualScanner.Text(), msgAndArgs...)
		case asOk:
			t.Errorf("didn't expect more lines but got: %s", actualScanner.Text())
			ok = false
		case esOk:
			t.Errorf("didn't got line but expected it to match against: %s", expectedScanner.Text())
			ok = false
		default:
			return ok
		}
	}
}

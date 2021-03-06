// This command builds a custom fn server with extensions compiled into it.
//
// NOTES:
// * We could just add extensions in the imports, but then there's no way to order them or potentially add extra config (although config should almost always be via env vars)

package main

import (
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"

	"github.com/urfave/cli"
	yaml "gopkg.in/yaml.v2"
)

func buildServer() cli.Command {
	cmd := buildServerCmd{}
	flags := append([]cli.Flag{}, cmd.flags()...)
	return cli.Command{
		Name:   "build-server",
		Usage:  "build custom fn server",
		Flags:  flags,
		Action: cmd.buildServer,
	}
}

type buildServerCmd struct {
	verbose bool
	noCache bool
}

func (b *buildServerCmd) flags() []cli.Flag {
	return []cli.Flag{
		cli.BoolFlag{
			Name:        "v",
			Usage:       "verbose mode",
			Destination: &b.verbose,
		},
		cli.BoolFlag{
			Name:        "no-cache",
			Usage:       "Don't use docker cache",
			Destination: &b.noCache,
		},
		cli.StringFlag{
			Name:  "tag,t",
			Usage: "image name and optional tag",
		},
	}
}

// steps:
// • Yaml file with extensions listed
// • NO‎TE: All extensions should use env vars for config
// • ‎Generating main.go with extensions
// * Generate a Dockerfile that gets all the extensions (using dep)
// • ‎then generate a main.go with extensions
// • ‎compile, throw in another container like main dockerfile
func (b *buildServerCmd) buildServer(c *cli.Context) error {

	if c.String("tag") == "" {
		return errors.New("docker tag required")
	}

	// path, err := os.Getwd()
	// if err != nil {
	// 	return err
	// }
	fpath := "ext.yaml"
	bb, err := ioutil.ReadFile(fpath)
	if err != nil {
		return fmt.Errorf("could not open %s for parsing. Error: %v", fpath, err)
	}
	ef := &extFile{}
	err = yaml.Unmarshal(bb, ef)
	if err != nil {
		return err
	}

	err = os.MkdirAll("tmp", 0777)
	if err != nil {
		return err
	}
	err = os.Chdir("tmp")
	if err != nil {
		return err
	}
	err = generateMain(ef)
	if err != nil {
		return err
	}
	err = generateDockerfile()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	err = runBuild(dir, c.String("tag"), "Dockerfile", b.noCache)
	if err != nil {
		return err
	}
	fmt.Printf("Custom Fn server built successfully.\n")
	return nil
}

func generateMain(ef *extFile) error {
	tmpl, err := template.New("main").Parse(mainTmpl)
	if err != nil {
		return err
	}
	f, err := os.Create("main.go")
	if err != nil {
		return err
	}
	defer f.Close()
	err = tmpl.Execute(f, ef)
	if err != nil {
		return err
	}
	return nil
}

func generateDockerfile() error {
	if err := ioutil.WriteFile("Dockerfile", []byte(dockerFileTmpl), os.FileMode(0644)); err != nil {
		return err
	}
	return nil
}

type extFile struct {
	Extensions []*extInfo `yaml:"extensions"`
}

type extInfo struct {
	Name string `yaml:"name"`
	// will have version and other things down the road
}

var mainTmpl = `package main

import (
	"context"

	"github.com/fnproject/fn/api/server"
	
	{{- range .Extensions }}
		_ "{{ .Name }}"
	{{- end}}
)

func main() {
	ctx := context.Background()
	funcServer := server.NewFromEnv(ctx)
	{{- range .Extensions }}
		funcServer.AddExtensionByName("{{ .Name }}")
	{{- end}}
	funcServer.Start(ctx)
}
`

// NOTE: Getting build errors with dep, probably because our vendor dir is wack. Might work again once we switch to dep.
// vendor/github.com/fnproject/fn/api/agent/drivers/docker/registry.go:93: too many arguments in call to client.NewRepository
// have ("context".Context, reference.Named, string, http.RoundTripper) want (reference.Named, string, http.RoundTripper)
// go build github.com/x/y/vendor/github.com/rdallman/migrate/database/mysql: no buildable Go source files in /go/src/github.com/x/y/vendor/github.com/rdallman/migrate/database/mysql
// # github.com/x/y/vendor/github.com/openzipkin/zipkin-go-opentracing/thrift/gen-go/scribe
// vendor/github.com/openzipkin/zipkin-go-opentracing/thrift/gen-go/scribe/scribe.go:210: undefined: thrift.TClient
var dockerFileTmpl = `# build stage
FROM golang:1.9-alpine AS build-env
RUN apk --no-cache add build-base git bzr mercurial gcc
# RUN go get -u github.com/golang/dep/cmd/dep
ENV D=/go/src/github.com/x/y
ADD main.go $D/
RUN cd $D && go get
# RUN cd $D && dep init && dep ensure
RUN cd $D && go build -o fnserver && cp fnserver /tmp/

# final stage
FROM fnproject/dind
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build-env /tmp/fnserver /app/fnserver
CMD ["./fnserver"]
`

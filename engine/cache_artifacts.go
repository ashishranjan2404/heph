package engine

import (
	"context"
	"encoding/json"
	"heph/engine/artifacts"
	log "heph/hlog"
	"heph/targetspec"
	"heph/utils"
	"heph/utils/fs"
	"heph/utils/tar"
	"os"
	"os/exec"
	"strings"
	"time"
)

type outTarArtifact struct {
	Target *Target
	Output string
}

func (a outTarArtifact) Gen(ctx context.Context, gctx artifacts.GenContext) error {
	target := a.Target

	var paths fs.Paths
	if a.Output == targetspec.SupportFilesOutput {
		paths = target.ActualSupportFiles()
	} else {
		paths = target.ActualOutFiles().Name(a.Output)
	}
	log.Tracef("Creating archive %v %v", target.FQN, a.Output)

	files := make([]tar.TarFile, 0)
	for _, file := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}

		file := file.WithRoot(gctx.OutRoot)

		files = append(files, tar.TarFile{
			From: file.Abs(),
			To:   file.RelRoot(),
		})
	}

	err := tar.Tar(ctx, files, gctx.ArtifactPath)
	if err != nil {
		return err
	}

	return nil
}

type hashOutputArtifact struct {
	Engine *Engine
	Target *Target
	Output string
}

func (a hashOutputArtifact) Gen(ctx context.Context, gctx artifacts.GenContext) error {
	outputHash := a.Engine.hashOutput(a.Target, a.Output)

	return fs.WriteFileSync(gctx.ArtifactPath, []byte(outputHash), os.ModePerm)
}

type hashInputArtifact struct {
	Engine *Engine
	Target *Target
}

func (a hashInputArtifact) Gen(ctx context.Context, gctx artifacts.GenContext) error {
	inputHash := a.Engine.hashInput(a.Target)

	return fs.WriteFileSync(gctx.ArtifactPath, []byte(inputHash), os.ModePerm)
}

type logArtifact struct{}

func (a logArtifact) Gen(ctx context.Context, gctx artifacts.GenContext) error {
	if gctx.LogFilePath == "" {
		return artifacts.Skip
	}

	return tar.Tar(ctx, []tar.TarFile{{
		From: gctx.LogFilePath,
		To:   "log.txt",
	}}, gctx.ArtifactPath)
}

type manifestArtifact struct {
	Engine *Engine
	Target *Target
}

type ManifestData struct {
	GitCommit  string                       `json:"git_commit,omitempty"`
	GitRef     string                       `json:"git_ref,omitempty"`
	InputHash  string                       `json:"input_hash,omitempty"`
	DepsHashes map[string]map[string]string `json:"deps_hashes,omitempty"`
	OutHashes  map[string]string            `json:"out_hashes,omitempty"`
	Timestamp  time.Time                    `json:"timestamp"`
}

func (a manifestArtifact) git(args ...string) string {
	cmd := exec.Command("git", args...)
	b, _ := cmd.Output()

	return strings.TrimSpace(string(b))
}

var gitCommitOnce utils.Once[string]
var gitRefOnce utils.Once[string]

func (a manifestArtifact) Gen(ctx context.Context, gctx artifacts.GenContext) error {
	d := ManifestData{
		GitCommit: gitCommitOnce.MustDo(func() (string, error) {
			return a.git("rev-parse", "HEAD"), nil
		}),
		GitRef: gitRefOnce.MustDo(func() (string, error) {
			return a.git("rev-parse", "--abbrev-ref", "HEAD"), nil
		}),
		InputHash:  a.Engine.hashInput(a.Target),
		DepsHashes: map[string]map[string]string{},
		OutHashes:  map[string]string{},
		Timestamp:  time.Now(),
	}

	e := a.Engine

	allDeps := a.Target.Deps.Merge(a.Target.Deps)
	for _, dep := range allDeps.All().Targets {
		if !dep.Target.Out.HasName(dep.Output) {
			continue
		}
		if d.DepsHashes[dep.Target.FQN] == nil {
			d.DepsHashes[dep.Target.FQN] = map[string]string{}
		}
		d.DepsHashes[dep.Target.FQN][dep.Output] = e.hashOutput(e.Targets.Find(dep.Target.FQN), dep.Output)
	}

	for _, name := range a.Target.OutWithSupport.Names() {
		d.OutHashes[name] = e.hashOutput(a.Target, name)
	}

	b, err := json.Marshal(d)
	if err != nil {
		return err
	}

	return fs.WriteFileSync(gctx.ArtifactPath, b, os.ModePerm)
}

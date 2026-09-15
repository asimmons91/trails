package cloudtask

import (
	"io/fs"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/jobs"
)

var _ trails.Spur = (*Backend)(nil)

func (b *Backend) ViewFS() fs.FS   { return nil }
func (b *Backend) AssetsFS() fs.FS { return nil }

func (b *Backend) Routes(g *trails.Group) {
	g.Post(b.pushPath, b.handlePush)
	g.Post(b.cleanupPath, b.handleCleanupPush)
}

// Jobs registers no job kinds of its own, same as dbqueue: this backend is
// infrastructure, not a job source.
func (b *Backend) Jobs(r *jobs.Registry) {}

package ast

import (
	"fmt"
	"os"
	"sync"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"golang.org/x/sync/errgroup"
)

type TaskfileGraph struct {
	sync.Mutex
	graph.Graph[string, *TaskfileVertex]
}

// A TaskfileVertex is a vertex on the Taskfile DAG.
type TaskfileVertex struct {
	URI      string
	Taskfile *Taskfile
}

func taskfileHash(vertex *TaskfileVertex) string {
	return vertex.URI
}

func NewTaskfileGraph() *TaskfileGraph {
	return &TaskfileGraph{
		sync.Mutex{},
		graph.New(taskfileHash,
			graph.Directed(),
			graph.PreventCycles(),
			graph.Rooted(),
		),
	}
}

func (tfg *TaskfileGraph) Visualize(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	return draw.DOT(tfg.Graph, f)
}

func (tfg *TaskfileGraph) Merge() (*Taskfile, error) {
	hashes, err := graph.TopologicalSort(tfg.Graph)
	if err != nil {
		return nil, err
	}

	predecessorMap, err := tfg.PredecessorMap()
	if err != nil {
		return nil, err
	}

	// Loop over each vertex in reverse topological order except for the root vertex.
	for i := len(hashes) - 1; i > 0; i-- {
		hash := hashes[i]

		includedVertex, err := tfg.Vertex(hash)
		if err != nil {
			return nil, err
		}

		var g errgroup.Group

		// Loop over edges leading to the current vertex
		for _, edge := range predecessorMap[hash] {
			g.Go(func() error {
				vertex, err := tfg.Vertex(edge.Source)
				if err != nil {
					return err
				}

				includes, ok := edge.Properties.Data.([]*Include)
				if !ok {
					return fmt.Errorf("task: Failed to get merge options")
				}

				for _, include := range includes {
					tfg.Lock()
					mergeErr := vertex.Taskfile.Merge(includedVertex.Taskfile, include)
					tfg.Unlock()

					if mergeErr != nil {
						return mergeErr
					}
				}
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return nil, err
		}
	}

	rootVertex, err := tfg.Vertex(hashes[0])
	if err != nil {
		return nil, err
	}

	return rootVertex.Taskfile, nil
}

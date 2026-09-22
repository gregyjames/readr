package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractWikilinks(t *testing.T) {
	text := `
Here is a note linking to [[Golang]] and [[Distributed Systems|DistSys]].
There is also another reference to [[golang]] and [[Microservices|Services]].
`
	links := ExtractWikilinks(text)
	assert.Len(t, links, 4)

	assert.Equal(t, "Golang", links[0].Target)
	assert.Equal(t, "Golang", links[0].Display)

	assert.Equal(t, "Distributed Systems", links[1].Target)
	assert.Equal(t, "DistSys", links[1].Display)

	assert.Equal(t, "golang", links[2].Target)
	assert.Equal(t, "golang", links[2].Display)

	assert.Equal(t, "Microservices", links[3].Target)
	assert.Equal(t, "Services", links[3].Display)
}

func TestExtractUniqueWikilinkTargets(t *testing.T) {
	text := `
First link to [[Kubernetes]].
Second link to [[kubernetes|k8s]].
Third link to [[Docker]].
Another [[KUBERNETES]].
`
	targets := ExtractUniqueWikilinkTargets(text)
	assert.Len(t, targets, 2)
	assert.Equal(t, "Kubernetes", targets[0])
	assert.Equal(t, "Docker", targets[1])

	assert.Empty(t, ExtractUniqueWikilinkTargets(""))
	assert.Empty(t, ExtractUniqueWikilinkTargets("Plain text without links."))
}

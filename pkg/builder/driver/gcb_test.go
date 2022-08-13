package driver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadStep(t *testing.T) {
	gcb := GCB{}
	r, err := gcb.GetRun("")
	require.NotNil(t, r)
	require.Error(t, err)
}

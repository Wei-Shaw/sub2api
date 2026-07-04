package admin

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"
)

func TestGroupRequestValidationAcceptsKiroPlatform(t *testing.T) {
	createReq := CreateGroupRequest{Name: "kiro-default", Platform: "kiro"}
	require.NoError(t, binding.Validator.ValidateStruct(createReq))

	updateReq := UpdateGroupRequest{Platform: "kiro"}
	require.NoError(t, binding.Validator.ValidateStruct(updateReq))
}

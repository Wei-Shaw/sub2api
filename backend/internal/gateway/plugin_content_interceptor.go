package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

const pluginContentCheckTimeout = 5 * time.Second

// PluginContentInterceptor adapts a plugin's ContentInterceptExtension gRPC
// service to the ContentInterceptor interface used by GatewayPipeline.
type PluginContentInterceptor struct {
	pluginName string
	conn       *grpc.ClientConn
	stub       pb.ContentInterceptExtensionClient
}

// NewPluginContentInterceptor creates an interceptor backed by a plugin's
// gRPC ContentInterceptExtension service.
func NewPluginContentInterceptor(pluginName string, conn *grpc.ClientConn) *PluginContentInterceptor {
	return &PluginContentInterceptor{
		pluginName: pluginName,
		conn:       conn,
		stub:       pb.NewContentInterceptExtensionClient(conn),
	}
}

func (p *PluginContentInterceptor) Check(ctx context.Context, c *gin.Context, req *ForwardRequest) (*ContentCheckResult, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, pluginContentCheckTimeout)
	defer cancel()

	pbReq := p.buildRequest(c, req)
	resp, err := p.stub.Check(rpcCtx, pbReq)
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.Unimplemented {
			return nil, nil
		}
		slog.Warn("content_intercept.plugin_check_failed",
			"plugin", p.pluginName,
			"error", err,
		)
		return nil, nil
	}

	if resp.GetAction() == pb.ContentAction_CONTENT_ACTION_ALLOW {
		return nil, nil
	}

	return &ContentCheckResult{
		Blocked:         resp.GetBlocked(),
		StatusCode:      int(resp.GetStatusCode()),
		Message:         resp.GetMessage(),
		HighestCategory: resp.GetHighestCategory(),
		HighestScore:    resp.GetHighestScore(),
	}, nil
}

func (p *PluginContentInterceptor) buildRequest(c *gin.Context, req *ForwardRequest) *pb.ContentCheckRequest {
	pbReq := &pb.ContentCheckRequest{
		RequestId: req.RequestID,
		Model:     req.Model,
		Protocol:  req.Protocol,
		Body:      req.RawBody,
	}

	if req.APIKey != nil {
		pbReq.ApiKeyId = req.APIKey.ID
		pbReq.ApiKeyName = req.APIKey.Name
		if req.APIKey.User != nil {
			pbReq.UserEmail = req.APIKey.User.Email
		}
		if req.APIKey.GroupID != nil {
			pbReq.GroupId = *req.APIKey.GroupID
		}
		if req.APIKey.Group != nil {
			pbReq.GroupName = req.APIKey.Group.Name
			pbReq.Provider = req.APIKey.Group.Platform
		}
	}

	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		pbReq.UserId = subject.UserID
	}

	pbReq.Endpoint = c.Request.URL.Path

	return pbReq
}

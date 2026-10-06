package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

func cacheWriteRequestEpoch(request *service.OpenAICacheWriteRequest) uint64 {
	if request == nil {
		return 0
	}
	return request.Epoch
}

package service

import "errors"

var ErrInvalidCostCenterAmount = errors.New("cost center amount must be positive")
var ErrInvalidCostCenterStatus = errors.New("invalid cost center event status")

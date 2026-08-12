package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	connect "connectrpc.com/connect"
	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/internal/evidence"
)

const (
	maxEvidenceCaseBytes    = 256 << 10
	maxEvidencePackageBytes = 8 << 20
)

type connectEvidenceHandler struct{ server *Server }

func (h *connectEvidenceHandler) ExportEvidencePackage(ctx context.Context, req *connect.Request[pb.ExportEvidencePackageRequest]) (*connect.Response[pb.ExportEvidencePackageResponse], error) {
	if len(req.Msg.GetTransferIds()) == 0 || len(req.Msg.GetTransferIds()) > 250 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("select between 1 and 250 transfers for an evidence package"))
	}
	if len(req.Msg.GetCaseJson()) > maxEvidenceCaseBytes {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid evidence package selection"))
	}
	var caseValue map[string]any
	if err := json.Unmarshal([]byte(req.Msg.GetCaseJson()), &caseValue); err != nil || len(caseValue) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid case file"))
	}
	runtime, err := h.server.network(req.Msg.GetNetwork())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	exported, err := runtime.Engine.ExportEvidence(ctx, req.Msg.GetTransferIds())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	packageJSON, err := evidence.Marshal([]byte(req.Msg.GetCaseJson()), exported, time.Now().UTC())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create evidence package: %w", err))
	}
	if len(packageJSON) > maxEvidencePackageBytes {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("evidence package is too large; select fewer transfers"))
	}
	return connect.NewResponse(&pb.ExportEvidencePackageResponse{PackageJson: string(packageJSON)}), nil
}

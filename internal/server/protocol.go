package server

import (
	"errors"
	"fmt"

	"github.com/aden-cs/parity-ot/internal/document"
	"github.com/aden-cs/parity-ot/internal/ot"
	collabpb "github.com/aden-cs/parity-ot/proto"
)

func operationFromProto(cpbOp *collabpb.Operation) (ot.Operation, error) {
	if cpbOp == nil {
		return ot.Operation{}, errors.New("missing operation")
	}

	components := make([]ot.Component, len(cpbOp.GetComponents()))
	for i, c := range cpbOp.GetComponents() {
		switch k := c.GetKind().(type) {
		case *collabpb.Component_Retain:
			components[i] = ot.Retain(k.Retain)
		case *collabpb.Component_Delete:
			components[i] = ot.Delete(k.Delete)
		case *collabpb.Component_Insert:
			components[i] = ot.Insert(k.Insert)
		default:
			return ot.Operation{}, fmt.Errorf("component %d has no kind", i)
		}
	}

	return ot.Operation{
		Components:   components,
		BaseLength:   cpbOp.GetBaseLength(),
		TargetLength: cpbOp.GetTargetLength(),
	}, nil
}

func operationToProto(op ot.Operation) *collabpb.Operation {
	components := make([]*collabpb.Component, len(op.Components))
	for i, c := range op.Components {
		switch c.Kind() {
		case ot.RetainKind:
			components[i] = &collabpb.Component{Kind: &collabpb.Component_Retain{Retain: c.Count()}}
		case ot.DeleteKind:
			components[i] = &collabpb.Component{Kind: &collabpb.Component_Delete{Delete: c.Count()}}
		case ot.InsertKind:
			components[i] = &collabpb.Component{Kind: &collabpb.Component_Insert{Insert: c.Text()}}
		default:
			panic(fmt.Sprintf("operationToProto: component %d has invalid kind %d", i, c.Kind()))
		}
	}

	return &collabpb.Operation{
		Components:   components,
		BaseLength:   op.BaseLength,
		TargetLength: op.TargetLength,
	}
}

func committedOperationToProto(op document.CommittedOperation) *collabpb.ServerMessage {
	return &collabpb.ServerMessage{
		Kind: &collabpb.ServerMessage_Committed{
			Committed: &collabpb.Committed{
				Revision:    op.Revision,
				ClientId:    op.ClientID,
				OperationId: op.OperationID,
				Operation:   operationToProto(op.Operation),
			},
		},
	}
}

func errorToProto(err error, opID string) *collabpb.ServerMessage {
	var code collabpb.ErrorCode
	switch {
	case errors.Is(err, document.ErrInvalidOperation):
		code = collabpb.ErrorCode_ERROR_CODE_INVALID_OPERATION
	case errors.Is(err, document.ErrBaseRevisionAhead):
		code = collabpb.ErrorCode_ERROR_CODE_INVALID_REVISION
	case errors.Is(err, document.ErrOperationIDReused):
		code = collabpb.ErrorCode_ERROR_CODE_OPERATION_ID_REUSED
	default:
		code = collabpb.ErrorCode_ERROR_CODE_BAD_REQUEST
	}

	return &collabpb.ServerMessage{
		Kind: &collabpb.ServerMessage_Error{
			Error: &collabpb.Error{
				Code:        code,
				Message:     err.Error(),
				OperationId: opID,
			},
		},
	}
}

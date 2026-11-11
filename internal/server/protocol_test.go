package server

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/aden-cs/parity-ot/internal/document"
	"github.com/aden-cs/parity-ot/internal/ot"
	collabpb "github.com/aden-cs/parity-ot/proto"
)

func pbRetain(n uint32) *collabpb.Component {
	return &collabpb.Component{Kind: &collabpb.Component_Retain{Retain: n}}
}

func pbDelete(n uint32) *collabpb.Component {
	return &collabpb.Component{Kind: &collabpb.Component_Delete{Delete: n}}
}

func pbInsert(s string) *collabpb.Component {
	return &collabpb.Component{Kind: &collabpb.Component_Insert{Insert: s}}
}

// conversionCases are valid pairs that must convert in both directions.
var conversionCases = []struct {
	name string
	pb   *collabpb.Operation
	op   ot.Operation
}{
	{
		name: "empty",
		pb:   &collabpb.Operation{},
		op:   ot.Operation{Components: []ot.Component{}},
	},
	{
		name: "retain insert delete",
		pb: &collabpb.Operation{
			Components:   []*collabpb.Component{pbRetain(3), pbInsert("abc"), pbDelete(2)},
			BaseLength:   5,
			TargetLength: 6,
		},
		op: ot.Operation{
			Components:   []ot.Component{ot.Retain(3), ot.Insert("abc"), ot.Delete(2)},
			BaseLength:   5,
			TargetLength: 6,
		},
	},
	{
		name: "multi-byte insert",
		pb: &collabpb.Operation{
			Components:   []*collabpb.Component{pbInsert("héllo 世界 👋")},
			BaseLength:   0,
			TargetLength: 10,
		},
		op: ot.Operation{
			Components:   []ot.Component{ot.Insert("héllo 世界 👋")},
			BaseLength:   0,
			TargetLength: 10,
		},
	},
	{
		// Conversion doesn't validate; Document.Submit does. Wrong lengths
		// must pass through unchanged so Submit can report them.
		name: "lengths passed through without validation",
		pb: &collabpb.Operation{
			Components:   []*collabpb.Component{pbRetain(1)},
			BaseLength:   7,
			TargetLength: 9,
		},
		op: ot.Operation{
			Components:   []ot.Component{ot.Retain(1)},
			BaseLength:   7,
			TargetLength: 9,
		},
	},
}

func TestOperationFromProto(t *testing.T) {
	for _, tt := range conversionCases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := operationFromProto(tt.pb)
			if err != nil {
				t.Fatalf("operationFromProto: unexpected error: %v", err)
			}
			// Treat nil and empty component slices as equal.
			if len(got.Components) == 0 && len(tt.op.Components) == 0 {
				got.Components, tt.op.Components = nil, nil
			}
			if !reflect.DeepEqual(got, tt.op) {
				t.Errorf("operationFromProto =\n  %+v\nwant\n  %+v", got, tt.op)
			}
		})
	}
}

func TestOperationFromProtoRejectsMalformed(t *testing.T) {
	tests := []struct {
		name string
		pb   *collabpb.Operation
	}{
		{name: "nil operation", pb: nil},
		{
			name: "nil component",
			pb:   &collabpb.Operation{Components: []*collabpb.Component{pbRetain(1), nil}},
		},
		{
			name: "component with no kind set",
			pb:   &collabpb.Operation{Components: []*collabpb.Component{{}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := operationFromProto(tt.pb); err == nil {
				t.Error("operationFromProto: expected error, got nil")
			}
		})
	}
}

func TestOperationToProto(t *testing.T) {
	for _, tt := range conversionCases {
		t.Run(tt.name, func(t *testing.T) {
			got := operationToProto(tt.op)
			if !proto.Equal(got, tt.pb) {
				t.Errorf("operationToProto =\n  %v\nwant\n  %v", got, tt.pb)
			}
		})
	}
}

func TestOperationRoundTrip(t *testing.T) {
	for _, tt := range conversionCases {
		t.Run(tt.name, func(t *testing.T) {
			// Through the wire format too, to catch anything that only
			// survives in-memory.
			wire, err := proto.Marshal(operationToProto(tt.op))
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var decoded collabpb.Operation
			if err := proto.Unmarshal(wire, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			got, err := operationFromProto(&decoded)
			if err != nil {
				t.Fatalf("operationFromProto: %v", err)
			}
			if len(got.Components) == 0 && len(tt.op.Components) == 0 {
				got.Components, tt.op.Components = nil, nil
			}
			if !reflect.DeepEqual(got, tt.op) {
				t.Errorf("round trip =\n  %+v\nwant\n  %+v", got, tt.op)
			}
		})
	}
}

func TestCommittedOperationToProto(t *testing.T) {
	committed := document.CommittedOperation{
		Revision:    42,
		ClientID:    "client-a",
		OperationID: "op-7",
		Operation: ot.Operation{
			Components:   []ot.Component{ot.Retain(2), ot.Insert("x"), ot.Delete(1)},
			BaseLength:   3,
			TargetLength: 3,
		},
	}
	want := &collabpb.ServerMessage{
		Kind: &collabpb.ServerMessage_Committed{Committed: &collabpb.Committed{
			Revision:    42,
			ClientId:    "client-a",
			OperationId: "op-7",
			Operation: &collabpb.Operation{
				Components:   []*collabpb.Component{pbRetain(2), pbInsert("x"), pbDelete(1)},
				BaseLength:   3,
				TargetLength: 3,
			},
		}},
	}

	got := committedOperationToProto(committed)
	if !proto.Equal(got, want) {
		t.Errorf("committedOperationToProto =\n  %v\nwant\n  %v", got, want)
	}
}

func TestErrorToProto(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want collabpb.ErrorCode
	}{
		{
			name: "invalid operation",
			err:  fmt.Errorf("%w: %w", document.ErrInvalidOperation, errors.New("base length 3, document length 5")),
			want: collabpb.ErrorCode_ERROR_CODE_INVALID_OPERATION,
		},
		{
			name: "base revision ahead",
			err:  fmt.Errorf("%w: base revision 9, current revision 4", document.ErrBaseRevisionAhead),
			want: collabpb.ErrorCode_ERROR_CODE_INVALID_REVISION,
		},
		{
			name: "operation ID reused",
			err:  fmt.Errorf("%w: client %q, op %q was committed at revision 2", document.ErrOperationIDReused, "c", "o"),
			want: collabpb.ErrorCode_ERROR_CODE_OPERATION_ID_REUSED,
		},
		{
			name: "unwrapped sentinel",
			err:  document.ErrInvalidOperation,
			want: collabpb.ErrorCode_ERROR_CODE_INVALID_OPERATION,
		},
		{
			name: "unknown error",
			err:  errors.New("component has no kind"),
			want: collabpb.ErrorCode_ERROR_CODE_BAD_REQUEST,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := errorToProto(tt.err, "op-1")

			e := msg.GetError()
			if e == nil {
				t.Fatalf("errorToProto: ServerMessage kind is %T, want *ServerMessage_Error", msg.GetKind())
			}
			if e.GetCode() != tt.want {
				t.Errorf("code = %v, want %v", e.GetCode(), tt.want)
			}
			if e.GetOperationId() != "op-1" {
				t.Errorf("operation_id = %q, want %q", e.GetOperationId(), "op-1")
			}
			if !strings.Contains(e.GetMessage(), tt.err.Error()) {
				t.Errorf("message = %q, want it to contain %q", e.GetMessage(), tt.err.Error())
			}
		})
	}
}

func TestErrorToProtoEmptyOperationID(t *testing.T) {
	// Errors unrelated to a submit (e.g. a bad Join) have no operation ID.
	e := errorToProto(errors.New("bad join"), "").GetError()
	if e == nil {
		t.Fatal("errorToProto: expected Error message")
	}
	if e.GetOperationId() != "" {
		t.Errorf("operation_id = %q, want empty", e.GetOperationId())
	}
}

// Copyright 2022-2023 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package unwind

import (
	"fmt"

	"github.com/parca-dev/parca-agent/internal/dwarf/frame"
)

type BpfCfaType uint16

// Constants are just to denote the rule type of calculation we do
// i.e whether we should compute based on rbp or rsp
const (
	//nolint: deadcode,varcheck
	// iota assigns a value to constants automatically
	cfaTypeUndefined BpfCfaType = iota
	cfaTypeRbp
	cfaTypeRsp
	cfaTypeExpression
	cfaTypeEndFdeMarker
	// cfaTypeFp cfa type is likely not defined for fp for arm64
	cfaTypeSp // for arm64
	cfaTypeLr // for arm64
)

type bpfRbpType uint16

const (
	rbpRuleOffsetUnchanged bpfRbpType = iota
	rbpRuleOffset
	rbpRuleRegister
	rbpTypeExpression
	rbpTypeUndefinedReturnAddress
	fpRuleOffset //for arm64 TODO(sylfrena): Maybe rbp can be used for both arm64 and x86? Discuss
)

// CompactUnwindTableRows encodes unwind information using 2x 64 bit words.
type CompactUnwindTableRow struct {
	pc        uint64
	lrOffset  int16 // link register for arm64; ignore for x86
	cfaType   uint8
	rbpType   uint8
	cfaOffset int16
	rbpOffset int16
}

func (cutr *CompactUnwindTableRow) Pc() uint64 {
	return cutr.pc
}

/*func (cutr *CompactUnwindTableRow) ReservedDoNotUse() uint16 {
	return cutr._reservedDoNotUse
}*/

func (cutr *CompactUnwindTableRow) LrOffset() int16 {
	return cutr.lrOffset
}

func (cutr *CompactUnwindTableRow) CfaType() uint8 {
	return cutr.cfaType
}

func (cutr *CompactUnwindTableRow) RbpType() uint8 {
	return cutr.rbpType
}

func (cutr *CompactUnwindTableRow) CfaOffset() int16 {
	return cutr.cfaOffset
}

func (cutr *CompactUnwindTableRow) RbpOffset() int16 {
	return cutr.rbpOffset
}

func (cutr *CompactUnwindTableRow) IsEndOfFDEMarker() bool {
	return cutr.cfaType == uint8(cfaTypeEndFdeMarker)
}

type CompactUnwindTable []CompactUnwindTableRow

func (t CompactUnwindTable) Len() int           { return len(t) }
func (t CompactUnwindTable) Less(i, j int) bool { return t[i].pc < t[j].pc }
func (t CompactUnwindTable) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }

// BuildCompactUnwindTable produces a compact unwind table for the given
// frame description entries.
func BuildCompactUnwindTable(fdes frame.FrameDescriptionEntries) (CompactUnwindTable, error) {
	table := make(CompactUnwindTable, 0, 4*len(fdes)) // heuristic: we expect each function to have ~4 unwind entries.
	for _, fde := range fdes {
		frameContext := frame.ExecuteDwarfProgram(fde, nil)
		for insCtx := frameContext.Next(); frameContext.HasNext(); insCtx = frameContext.Next() {
			row := unwindTableRow(insCtx)
			compactRow, err := rowToCompactRow(row)
			if err != nil {
				return CompactUnwindTable{}, err
			}
			table = append(table, compactRow)
		}
		// Add a synthetic row for the end of the function.
		table = append(table, CompactUnwindTableRow{
			pc:      fde.End(),
			cfaType: uint8(cfaTypeEndFdeMarker),
		})
	}
	return table, nil
}

// rowToCompactRow converts an unwind row to a compact row.
func rowToCompactRow(row *UnwindTableRow) (CompactUnwindTableRow, error) {
	var cfaType uint8
	var rbpType uint8
	var cfaOffset int16
	var rbpOffset int16
	var LrOffset int16 = 0

	// CFA.
	//nolint:exhaustive
	switch row.CFA.Rule {
	case frame.RuleCFA:
		// the values are just numbers and don't have a type so they can overlap and this won't work
		// if row.CFA.Reg == frame.X86_64FramePointer {
		// 	cfaType = uint8(cfaTypeRbp)
		// } else if row.CFA.Reg == frame.X86_64StackPointer {
		// 	cfaType = uint8(cfaTypeRsp)
		// } else
		// if row.CFA.Reg == frame.Arm64FramePointer {
		//	cfaType = uint8(cfaTypeFp) // TODO(sylfrena): check number of bytes
		//} else
		// cfa type only seems to be for sp, not fp in arm64
		if row.CFA.Reg == frame.Arm64StackPointer {
			cfaType = uint8(cfaTypeSp) // TODO(sylfrena): check bytes, it is just sp for arm64 btw
		}
		cfaOffset = int16(row.CFA.Offset)
		/*if row.CFA.Reg == frame.Arm64LinkRegister { // TODO(sylfrena): check if there is a rule for LR
			Lr = uint16(row.CFA.Offset) // TODO(sylfrena): check bytes
		}*/
		// for arm64: CFA rules are just for stack pointers, and not even for frame pointers(seems so far in case of arm64)
		// so link register doesn't matter here really
	case frame.RuleExpression:
		cfaType = uint8(cfaTypeExpression)
		cfaOffset = int16(ExpressionIdentifier(row.CFA.Expression))
	/*case frame.RuleRegister:
	if row.CFA.Reg == frame.Arm64LinkRegister {
		cfaType = uint8(cfaTypeLr)
		LrOffset = uint16(row.CFA.Offset)
	}*/
	default:
		return CompactUnwindTableRow{}, fmt.Errorf("CFA rule is not valid: %d", row.CFA.Rule)
	}

	// TODO(sylfrena): change for arm64
	// Frame pointer.
	switch row.RBP.Rule {
	case frame.RuleOffset:
		rbpType = uint8(fpRuleOffset) //uint8(rbpRuleOffset) // TODO(sylfrena): works only for arm64, fix for x86 also
		rbpOffset = int16(row.RBP.Offset)
		fmt.Println()
		// curious that the following condition doesn't satisfy. it should.
		// On further investigation, it doesn't because only Offset Rule is applied, and register value is x0, not x29
		if row.RBP.Reg == frame.Arm64FramePointer {
			fmt.Println("ruley fp offset")
			rbpType = uint8(fpRuleOffset)
		}
	case frame.RuleRegister:
		rbpType = uint8(rbpRuleRegister)
	case frame.RuleExpression:
		rbpType = uint8(rbpTypeExpression)
	case frame.RuleUndefined:
	case frame.RuleUnknown:
	case frame.RuleSameVal:
	case frame.RuleValOffset:
	case frame.RuleValExpression:
	case frame.RuleCFA:
	}

	// TODO(sylfrena): change for arm64
	// Return address.
	switch row.RA.Rule {
	case frame.RuleOffset:
		// for x86: rbpType = uint8(rbpTypeUndefinedReturnAddress)
		fmt.Println("ruley offset")
		LrOffset = int16(row.RA.Offset)
	case frame.RuleCFA:
		//fmt.Println("ruley cfa")
	case frame.RuleRegister:
	case frame.RuleUnknown:
		//fmt.Println("ruley unknown")
	case frame.RuleUndefined:
		//fmt.Println("ruley undefined")
	}

	return CompactUnwindTableRow{
		pc:        row.Loc,
		lrOffset:  LrOffset, //TODO(sylfrena): dummy value; add offset here instead
		cfaType:   cfaType,
		rbpType:   rbpType,
		cfaOffset: cfaOffset,
		rbpOffset: rbpOffset,
	}, nil
}

// CompactUnwindTableRepresentation converts an unwind table to its compact table
// representation.
func CompactUnwindTableRepresentation(unwindTable UnwindTable) (CompactUnwindTable, error) {
	compactTable := make(CompactUnwindTable, 0, len(unwindTable))

	for i := range unwindTable {
		row := unwindTable[i]

		compactRow, err := rowToCompactRow(&row)
		if err != nil {
			return CompactUnwindTable{}, err
		}

		compactTable = append(compactTable, compactRow)
	}

	return compactTable, nil
}

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestValidateEffectiveUserRefusesRootWithActionableGuidance(t *testing.T) {
	err := validateEffectiveUser(0)
	if err == nil {
		t.Fatal("validateEffectiveUser(0) = nil, want refusal")
	}
	message := err.Error()
	for _, want := range []string{"effective UID 0", "without sudo", "target user"} {
		if !strings.Contains(message, want) {
			t.Errorf("root refusal %q does not contain %q", message, want)
		}
	}
}

func TestValidateEffectiveUserAllowsUnprivilegedAndUnknownIDs(t *testing.T) {
	for _, euid := range []int{1, 501, 1000, -1} {
		if err := validateEffectiveUser(euid); err != nil {
			t.Errorf("validateEffectiveUser(%d) = %v", euid, err)
		}
	}
}

func TestMainChecksEffectiveUserBeforeDispatch(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var mainBody *ast.BlockStmt
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "main" {
			mainBody = function.Body
			break
		}
	}
	if mainBody == nil || len(mainBody.List) == 0 {
		t.Fatal("main function has no executable body")
	}
	guard, ok := mainBody.List[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("main first statement is %T, want effective-user guard", mainBody.List[0])
	}
	assignment, ok := guard.Init.(*ast.AssignStmt)
	if !ok || len(assignment.Rhs) != 1 {
		t.Fatalf("main first guard init is %T, want validation assignment", guard.Init)
	}
	validation, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || callName(validation.Fun) != "validateEffectiveUser" || len(validation.Args) != 1 {
		t.Fatalf("main first guard does not call validateEffectiveUser: %#v", assignment.Rhs[0])
	}
	effectiveID, ok := validation.Args[0].(*ast.CallExpr)
	if !ok || callName(effectiveID.Fun) != "effectiveUserID" {
		t.Fatalf("validateEffectiveUser argument is %#v, want effectiveUserID()", validation.Args[0])
	}
}

func callName(expression ast.Expr) string {
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

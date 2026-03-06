package topology

import "testing"

func TestKRLTopology_InterchangeAndReachability(t *testing.T) {
	graph, err := LoadKRLTopology()
	if err != nil {
		t.Fatalf("load topology: %v", err)
	}

	if !graph.IsInterchange("DU") {
		t.Fatalf("expected DU to be interchange")
	}
	if graph.IsInterchange("RW") {
		t.Fatalf("expected RW to not be interchange")
	}

	if !graph.CanReach("cikarang_via_mri", "DU", "SUD") {
		t.Fatalf("expected cikarang_via_mri to reach SUD from DU")
	}
	if graph.CanReach("tangerang", "RW", "SUD") {
		t.Fatalf("expected tangerang corridor to not reach SUD from RW")
	}
}

func TestKRLTopology_ClassifyRoute(t *testing.T) {
	graph, err := LoadKRLTopology()
	if err != nil {
		t.Fatalf("load topology: %v", err)
	}

	corridor, ok := graph.ClassifyRoute("CIKARANG-KAMPUNGBANDAN VIA MRI", []string{"SUD", "THB", "AK", "KPB"})
	if !ok {
		t.Fatalf("expected route classification")
	}
	if corridor != "cikarang_via_mri" {
		t.Fatalf("corridor = %q, want cikarang_via_mri", corridor)
	}
}

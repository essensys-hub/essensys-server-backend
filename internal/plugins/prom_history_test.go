package plugins

import "testing"

func TestParsePromQueryRange(t *testing.T) {
	raw := []byte(`{"status":"success","data":{"resultType":"matrix","result":[{"values":[[1700000000,"1.25"],[1700000300,"2.5"]]}]}}`)
	pts, err := parsePromMatrix(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 2 || pts[1].Value != 2.5 {
		t.Fatalf("%+v", pts)
	}
}

func TestParsePromMatrixEmptyResult(t *testing.T) {
	raw := []byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`)
	pts, err := parsePromMatrix(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 0 {
		t.Fatalf("attendu vide, got %+v", pts)
	}
}

func TestParsePromMatrixError(t *testing.T) {
	raw := []byte(`{"status":"error","errorType":"bad_data","error":"invalid query"}`)
	if _, err := parsePromMatrix(raw); err == nil {
		t.Fatal("attendu une erreur sur status=error")
	}
}

func TestParsePromMatrixInvalidJSON(t *testing.T) {
	if _, err := parsePromMatrix([]byte(`not json`)); err == nil {
		t.Fatal("attendu une erreur sur JSON invalide")
	}
}

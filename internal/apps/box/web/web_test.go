package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dgs-toolbox/internal/box/doctype"
	"dgs-toolbox/internal/box/tag"
)

func server(t *testing.T) *httptest.Server {
	t.Helper()
	source := NewSample("AUD")
	srv := httptest.NewServer(Handler(Settings{Root: "/Volumes/nas/Box", Inbox: "~/Scans", Currency: "AUD", Zone: "Australia/Sydney", Source: source}))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string, into any) {
	t.Helper()
	response, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", path, response.Status)
	}
	if into != nil {
		if err := json.NewDecoder(response.Body).Decode(into); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
}

func send(t *testing.T, srv *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	request, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := srv.Client().Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	out, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(out)
}

func TestThePagesAreServed(t *testing.T) {
	srv := server(t)
	for path, want := range map[string]string{
		"/intake/": "Intake",
		"/browse/": "Browse",
		"/check/":  "Exceptions",
	} {
		response, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("GET %s: %s", path, response.Status)
		}
		if !strings.Contains(string(body), want) {
			t.Errorf("GET %s does not look like the %s page", path, want)
		}
	}
	// The root goes to browse: the page that only reads is the safe landing.
	response, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	response.Body.Close()
	if response.Request.URL.Path != "/browse/" {
		t.Errorf("/ landed on %s, want /browse/", response.Request.URL.Path)
	}
}

func TestConfigSaysWhenTheScansAreMadeUp(t *testing.T) {
	var body pageConfig
	get(t, server(t), "/api/config", &body)
	if !body.Sample {
		t.Error("the sample source did not say it is a sample; a page must not let someone describe scans that do not exist")
	}
	if body.Root != "/Volumes/nas/Box" || body.Currency != "AUD" {
		t.Errorf("config = %+v", body)
	}
}

func TestTypesAreTheCatalogue(t *testing.T) {
	var body struct {
		Types []typeOption `json:"types"`
	}
	get(t, server(t), "/api/types", &body)
	if len(body.Types) != len(doctype.All()) {
		t.Fatalf("served %d types, the catalogue has %d", len(body.Types), len(doctype.All()))
	}
	keys := map[string]string{}
	for _, option := range body.Types {
		if option.Key == "" {
			t.Errorf("%s has no key", option.Name)
		}
		if other, taken := keys[option.Key]; taken {
			t.Errorf("%s and %s share the key %q", other, option.Name, option.Key)
		}
		keys[option.Key] = option.Name
	}
	// A ticket expires on the event date; a contract has no expiry at all.
	// Those are different answers, and the page draws them differently.
	for _, option := range body.Types {
		switch option.Name {
		case "ticket":
			if option.Lifetime == nil || *option.Lifetime != 0 {
				t.Errorf("ticket lifetime = %v, want 0 days", option.Lifetime)
			}
		case "contract":
			if option.Lifetime != nil {
				t.Errorf("contract lifetime = %v, want none", *option.Lifetime)
			}
		}
	}
}

func TestIntakeIsInScanTimeOrder(t *testing.T) {
	var body struct {
		Pending []Scan `json:"pending"`
	}
	get(t, server(t), "/api/intake", &body)
	if len(body.Pending) < 2 {
		t.Fatal("the sample inbox is empty")
	}
	for index := 1; index < len(body.Pending); index++ {
		if body.Pending[index-1].ScannedAt > body.Pending[index].ScannedAt {
			t.Fatalf("pending scans are not in scan-time order: %s then %s",
				body.Pending[index-1].ScannedAt, body.Pending[index].ScannedAt)
		}
	}
}

func TestScansCarryComputedStateAndPerCurrencyTotals(t *testing.T) {
	var body struct {
		Scans  []Scan `json:"scans"`
		Totals []struct {
			Currency string `json:"currency"`
			Display  string `json:"display"`
			Count    int    `json:"count"`
		} `json:"totals"`
	}
	get(t, server(t), "/api/scans", &body)
	if len(body.Scans) == 0 {
		t.Fatal("no scans served")
	}
	for _, scan := range body.Scans {
		if scan.State == "" {
			t.Errorf("%s has no computed state", scan.Digest)
		}
	}
	// Several currencies, each on its own line: a combined figure would need a
	// rate, which is not available offline.
	if len(body.Totals) < 2 {
		t.Fatalf("totals = %+v, want one line per currency", body.Totals)
	}
	seen := map[string]bool{}
	for _, total := range body.Totals {
		if seen[total.Currency] {
			t.Errorf("%s appears twice", total.Currency)
		}
		seen[total.Currency] = true
		if !strings.HasPrefix(total.Display, total.Currency) {
			t.Errorf("%q does not name its currency", total.Display)
		}
	}
	if !seen["AUD"] || !seen["JPY"] {
		t.Errorf("totals = %+v, want AUD and JPY apart", body.Totals)
	}
}

func TestAnUnknownTypeSurvivesBeingRead(t *testing.T) {
	var body struct {
		Scans []Scan `json:"scans"`
	}
	get(t, server(t), "/api/scans", &body)
	var found bool
	for _, scan := range body.Scans {
		if scan.Type == "warranty" {
			found = true
			if scan.TypeKnown {
				t.Error("a type this build does not register was reported as known")
			}
			if scan.State == "dead" {
				t.Error("an unknown type was declared dead")
			}
		}
	}
	if !found {
		t.Fatal("the sample no longer holds a scan with an unregistered type")
	}
}

func TestEditingRefusesWhatWouldBeWrong(t *testing.T) {
	srv := server(t)
	for _, test := range []struct{ name, body, wants string }{
		{"a type that does not exist", `{"digest":"33cc44dd","type":"tax-notice"}`, "no such type"},
		{"a date that is not one", `{"digest":"33cc44dd","eventDate":"03/11/2019"}`, "YYYY-MM-DD"},
		{"an offset instead of a zone", `{"digest":"33cc44dd","eventZone":"+10:00"}`, "IANA"},
		{"a grouped amount", `{"digest":"33cc44dd","total":"1,234"}`, "amount"},
		{"an unknown scan", `{"digest":"nope","description":"x"}`, "no scan"},
	} {
		status, body := send(t, srv, http.MethodPatch, "/api/scan", test.body)
		if status == http.StatusOK {
			t.Errorf("%s: accepted", test.name)
			continue
		}
		if !strings.Contains(body, test.wants) {
			t.Errorf("%s: error %q does not mention %q", test.name, strings.TrimSpace(body), test.wants)
		}
	}
}

func TestClearingAnExpiryKeepsAScanForGood(t *testing.T) {
	srv := server(t)
	// A cinema ticket whose event date has passed is dead...
	status, body := send(t, srv, http.MethodPatch, "/api/scan", `{"digest":"ccddeeff"}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH: %s %s", http.StatusText(status), body)
	}
	var scan Scan
	if err := json.Unmarshal([]byte(body), &scan); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if scan.State != "dead" {
		t.Fatalf("a past cinema ticket is %q, want dead", scan.State)
	}
	// ...until one key clears the expiry, and the type's same-day default must
	// not creep back.
	status, body = send(t, srv, http.MethodPatch, "/api/scan", `{"digest":"ccddeeff","expiryCleared":true}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH: %s %s", http.StatusText(status), body)
	}
	if err := json.Unmarshal([]byte(body), &scan); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if scan.State != "permanent" || scan.Expiry != "" {
		t.Errorf("after keeping it: state %q, expiry %q; want permanent and none", scan.State, scan.Expiry)
	}
}

func TestFilingEmptiesTheInboxOneScanAtATime(t *testing.T) {
	srv := server(t)
	var before struct {
		Pending []Scan `json:"pending"`
		Count   int    `json:"count"`
	}
	get(t, srv, "/api/intake", &before)
	// Nobody can say what this is, so it is filed as unsorted rather than left
	// behind: the inbox has to be drainable.
	status, body := send(t, srv, http.MethodPost, "/api/intake/file",
		`{"digest":"`+before.Pending[0].Digest+`","type":"unsorted","tags":[" japan ","travel","japan",""]}`)
	if status != http.StatusOK {
		t.Fatalf("file: %s %s", http.StatusText(status), body)
	}
	var after struct {
		Count int `json:"count"`
	}
	get(t, srv, "/api/intake", &after)
	if after.Count != before.Count-1 {
		t.Errorf("inbox went from %d to %d", before.Count, after.Count)
	}
	var filed struct {
		Scans []Scan `json:"scans"`
	}
	get(t, srv, "/api/scans", &filed)
	var seen bool
	for _, scan := range filed.Scans {
		if scan.Digest == before.Pending[0].Digest {
			seen = true
			if scan.Reviewed {
				t.Error("a scan filed at intake was marked as checked; an intake type is a guess")
			}
			if strings.Join(scan.Tags, ",") != "japan,travel" {
				t.Errorf("tags = %q, want normalized intake tags", scan.Tags)
			}
		}
	}
	if !seen {
		t.Error("the filed scan is not in the Box")
	}
}

func TestTrashTakesAScanOutOfTheBoxWithoutDeletingIt(t *testing.T) {
	srv := server(t)
	status, body := send(t, srv, http.MethodPost, "/api/trash", `{"digest":"33cc44dd","reason":"test"}`)
	if status != http.StatusOK {
		t.Fatalf("trash: %s %s", http.StatusText(status), body)
	}
	var filed struct {
		Scans []Scan `json:"scans"`
	}
	get(t, srv, "/api/scans", &filed)
	for _, scan := range filed.Scans {
		if scan.Digest == "33cc44dd" {
			t.Error("a trashed scan is still listed in the Box")
		}
	}
}

func TestImageNamesAPage(t *testing.T) {
	srv := server(t)
	// 44dd55ee is a six-page sample.
	for query, want := range map[string]int{
		"digest=44dd55ee&size=thumb&page=3": http.StatusOK,
		"digest=44dd55ee&size=thumb&page=7": http.StatusNotFound,
		"digest=44dd55ee&size=thumb&page=0": http.StatusBadRequest,
		"digest=44dd55ee&size=thumb&page=x": http.StatusBadRequest,
	} {
		response, err := srv.Client().Get(srv.URL + "/api/image?" + query)
		if err != nil {
			t.Fatalf("GET %s: %v", query, err)
		}
		response.Body.Close()
		if response.StatusCode != want {
			t.Errorf("%s: %s, want %d", query, response.Status, want)
		}
	}
}

func TestImageIsAskedForByDigestNotByPath(t *testing.T) {
	srv := server(t)
	response, err := srv.Client().Get(srv.URL + "/api/image?digest=33cc44dd&size=preview")
	if err != nil {
		t.Fatalf("GET image: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET image: %s", response.Status)
	}
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "svg") {
		t.Errorf("Content-Type = %q", got)
	}
	// A digest nobody knows is a miss, and there is no way to name a path at
	// all: a path in a request is a path out of the Box.
	missing, err := srv.Client().Get(srv.URL + "/api/image?digest=../../etc/passwd")
	if err != nil {
		t.Fatalf("GET image: %v", err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("a path as a digest answered %s, want 404", missing.Status)
	}
}

func TestExceptionsAreReportedNotRepaired(t *testing.T) {
	var body struct {
		Exceptions []Exception `json:"exceptions"`
	}
	get(t, server(t), "/api/exceptions", &body)
	kinds := map[string]Exception{}
	for _, exception := range body.Exceptions {
		kinds[exception.Kind] = exception
	}
	for _, kind := range []string{"no sidecar", "no scan", "digest mismatch"} {
		if _, found := kinds[kind]; !found {
			t.Errorf("no %q exception is reported", kind)
		}
	}
	if !kinds["no sidecar"].Adoptable {
		t.Error("a file put in the tree by hand should be adoptable, not only an error")
	}
	if kinds["digest mismatch"].Adoptable {
		t.Error("a digest mismatch must not offer a one-click fix: rewriting the sidecar erases the evidence")
	}
}

// The inbox reports what it held back. A skip nobody is told about means
// believing the inbox is empty when it is not.
func TestIncompleteIsItsOwnEndpoint(t *testing.T) {
	srv := server(t)
	var body struct {
		Count      int         `json:"count"`
		Incomplete []Exception `json:"incomplete"`
	}
	get(t, srv, "/api/incomplete", &body)
	if body.Count != len(body.Incomplete) || body.Count == 0 {
		t.Fatalf("incomplete: %+v", body)
	}
	if body.Incomplete[0].Detail == "" {
		t.Error("nothing said why it was held back")
	}
}

// A batch is one request, and a failure in it is per record.
func TestBatchEditIsOneRequest(t *testing.T) {
	srv := server(t)
	var intake struct {
		Pending []Scan `json:"pending"`
	}
	get(t, srv, "/api/intake", &intake)
	if len(intake.Pending) < 2 {
		t.Fatalf("not enough sample scans to batch: %d", len(intake.Pending))
	}
	digests := []string{intake.Pending[0].Digest, intake.Pending[1].Digest, "sha256:nothing"}
	payload, err := json.Marshal(map[string]any{
		"digests": digests,
		"edit":    map[string]any{"type": "receipt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, out := send(t, srv, http.MethodPatch, "/api/scans", string(payload))
	if status != http.StatusOK {
		t.Fatalf("PATCH /api/scans: %d %s", status, out)
	}
	var result BatchResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Changed) != 2 {
		t.Errorf("one missing scan undid the real ones: %+v", result)
	}
	if len(result.Failed) != 1 {
		t.Errorf("the failure was not reported on its own: %+v", result.Failed)
	}
}

func TestTagsAreCountedAcrossTheBox(t *testing.T) {
	srv := server(t)
	var body struct {
		Tags []tag.Use `json:"tags"`
	}
	get(t, srv, "/api/tags", &body)
	counts := map[string]int{}
	for _, use := range body.Tags {
		counts[use.Name] = use.Count
	}
	if counts["japan-2019"] < 2 || counts["keep"] < 1 {
		t.Fatalf("tags = %+v, want the sample's japan-2019 and keep", body.Tags)
	}
	if body.Tags[0].Name != "japan-2019" {
		t.Errorf("most used first: %+v", body.Tags)
	}
}

func TestABatchAddsTagsWithoutRemovingAny(t *testing.T) {
	srv := server(t)
	var body struct {
		Scans []Scan `json:"scans"`
	}
	get(t, srv, "/api/scans", &body)
	var target Scan
	for _, scan := range body.Scans {
		if len(scan.Tags) > 0 && len(scan.Documents) == 0 {
			target = scan
			break
		}
	}
	if target.Digest == "" {
		t.Fatal("no tagged sample scan to add to")
	}
	payload, err := json.Marshal(map[string]any{
		"digests": []string{target.Digest},
		"edit":    map[string]any{"addTags": []string{"Tax Return", target.Tags[0]}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status, out := send(t, srv, http.MethodPatch, "/api/scans", string(payload)); status != http.StatusOK {
		t.Fatalf("PATCH /api/scans: %d %s", status, out)
	}
	get(t, srv, "/api/scans", &body)
	for _, scan := range body.Scans {
		if scan.Digest != target.Digest {
			continue
		}
		want := strings.Join(append(append([]string{}, target.Tags...), "tax-return"), ",")
		if got := strings.Join(scan.Tags, ","); got != want {
			t.Errorf("tags = %q, want %q", got, want)
		}
	}
}

func TestABatchNamingNothingIsRefused(t *testing.T) {
	srv := server(t)
	if status, _ := send(t, srv, http.MethodPatch, "/api/scans", `{"digests":[]}`); status != http.StatusBadRequest {
		t.Errorf("an empty batch was accepted: %d", status)
	}
	if status, _ := send(t, srv, http.MethodPost, "/api/trash/batch", `{"digests":[]}`); status != http.StatusBadRequest {
		t.Errorf("an empty discard was accepted: %d", status)
	}
}

// The stand-in refuses to adopt or verify rather than pretending there are
// bytes behind it: a sample presented as a real Box is the one thing that must
// never happen.
func TestTheStandInRefusesWhatWouldNeedRealBytes(t *testing.T) {
	srv := server(t)
	if status, _ := send(t, srv, http.MethodPost, "/api/adopt", `{"path":"2026/2026-09-18/scan.pdf"}`); status != http.StatusBadRequest {
		t.Errorf("a made-up file was adopted: %d", status)
	}
	if status, _ := send(t, srv, http.MethodPost, "/api/verify", ``); status != http.StatusBadRequest {
		t.Errorf("made-up bytes were verified: %d", status)
	}
}

// A picture is revalidated, not kept blind: a redrawn picture under the same
// digest must reach the page, and an unchanged one costs a 304.
func TestImageIsRevalidated(t *testing.T) {
	srv := server(t)
	first, err := srv.Client().Get(srv.URL + "/api/image?digest=33cc44dd&size=preview")
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	tag := first.Header.Get("ETag")
	if tag == "" || first.Header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("headers: %v", first.Header)
	}
	request, _ := http.NewRequest("GET", srv.URL+"/api/image?digest=33cc44dd&size=preview", nil)
	request.Header.Set("If-None-Match", tag)
	again, err := srv.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	again.Body.Close()
	if again.StatusCode != http.StatusNotModified {
		t.Errorf("unchanged picture answered %d", again.StatusCode)
	}
}

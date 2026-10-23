- `{{.Clock}} {{.Offset}}`{{with .Content}} · {{.}}{{end}} {{.ID}}
    - {{.Address}}
    - {{.CopyLink}}
    - {{.MapLinks}}
{{- range .Images}}
    - ![[{{.}}]]
{{- end}}

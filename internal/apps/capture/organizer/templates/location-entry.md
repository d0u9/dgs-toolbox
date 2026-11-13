- `{{.Clock}} {{.Offset}}`{{with .ContentLines}} · {{index . 0}}{{end}} {{.ID}}
    - {{.Address}}
    - {{.CopyLink}}
    - {{.MapLinks}}
{{- range $index, $line := .ContentLines}}{{if $index}}
    - {{$line}}{{end}}{{end}}
{{- range .Images}}
    - ![[{{.}}]]
{{- end}}

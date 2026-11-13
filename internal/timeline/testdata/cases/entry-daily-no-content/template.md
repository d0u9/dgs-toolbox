- `{{.Clock}} {{.Offset}}`{{with .ContentLines}} · {{index . 0}}{{end}} {{.ID}}
    - {{.Address}}
{{- if .Latitude}}
    - {{.CopyLink "Copy Position"}}{{with .Altitude}}, {{.}}{{end}}
    - {{.MapLinks "Apple, 高德, Google"}}
{{- end}}
{{- range $index, $line := .ContentLines}}{{if $index}}
    - {{$line}}{{end}}{{end}}
{{- range .Images}}
    - ![[{{.}}]]
{{- end}}

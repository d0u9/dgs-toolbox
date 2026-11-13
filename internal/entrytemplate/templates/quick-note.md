- `{{.Clock}} {{.Offset}}`{{with .ContentLines}} · {{index . 0}}{{end}} {{.ID}}
{{- range $index, $line := .ContentLines}}{{if $index}}
    - {{$line}}{{end}}{{end}}

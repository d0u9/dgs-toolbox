- {{.When}} {{.ID}}
  - Content:
{{- range .ContentLines}}
    - {{.}}
{{- end}}
  - altitude: {{.Altitude}}
  - address: {{.Address}}

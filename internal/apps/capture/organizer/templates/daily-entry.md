- Date: {{.When}} {{.ID}}
{{- with .ContentLines}}
    - Content:
    {{- range .}}
        - {{.}}
    {{- end}}
{{- end}}
    - Coordinates: {{.Latitude}}, {{.Longitude}}{{with .Altitude}}, {{.}}{{end}}
    - Address: {{.Address}}

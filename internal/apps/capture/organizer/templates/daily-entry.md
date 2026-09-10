- Date: {{.When}} {{.ID}}
    - Content:
    {{- range .ContentLines}}
        - {{.}}
    {{- end}}
    - Coordinates: {{.Latitude}}, {{.Longitude}}{{with .Altitude}}, {{.}}{{end}}
    - Address: {{.Address}}

- Date: {{.When}} {{.ID}}
    - Content:
    {{- range .ContentLines}}
        - {{.}}
    {{- end}}
    - {{.Coordinates}}
    - Altitude: {{.Altitude}}
    - Address: {{.Address}}

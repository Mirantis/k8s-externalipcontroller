{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create IpClaimPool resource to provide IP pool for the IP claim allocator.
*/}}
{{- define "ipclaimpool" -}}
apiVersion: ipcontroller.ext/v1
kind: IpClaimPool
metadata:
    name: default-pool
spec:
    cidr: {{ .cidr }}
    {{ if .ranges }}
    {{ . | include "ipranges" }}
    {{- end -}}
{{- end -}}

{{/*
Define IP ranges list for IpClaimPool resource.
*/}}
{{- define "ipranges" -}}
    ranges:
        {{- range .ranges }}
        - - {{ index . 0 | quote }}
          - {{ index . 1 | quote }}
        {{- end }}
{{- end -}}

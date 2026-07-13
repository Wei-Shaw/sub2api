{{/*
Expand the chart name.
*/}}
{{- define "sub2api.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "sub2api.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "sub2api.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels.
*/}}
{{- define "sub2api.labels" -}}
helm.sh/chart: {{ include "sub2api.chart" . }}
{{ include "sub2api.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/*
Selector labels.
*/}}
{{- define "sub2api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sub2api.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Application ServiceAccount name.
*/}}
{{- define "sub2api.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "sub2api.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.appSecretName" -}}
{{- if .Values.secrets.app.existingSecret -}}
{{- .Values.secrets.app.existingSecret -}}
{{- else -}}
{{- printf "%s-app" (include "sub2api.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.image" -}}
{{- $registry := coalesce .Values.image.registry .Values.global.imageRegistry -}}
{{- $repository := .Values.image.repository -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" (trimSuffix "/" $registry) (trimPrefix "/" $repository) $tag -}}
{{- else -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.waitImage" -}}
{{- $registry := coalesce .Values.waitForDependencies.image.registry .Values.global.imageRegistry -}}
{{- $repository := .Values.waitForDependencies.image.repository -}}
{{- $tag := .Values.waitForDependencies.image.tag -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" (trimSuffix "/" $registry) (trimPrefix "/" $repository) $tag -}}
{{- else -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.mihomoImage" -}}
{{- $registry := coalesce .Values.mihomo.image.registry .Values.global.imageRegistry -}}
{{- $repository := .Values.mihomo.image.repository -}}
{{- $tag := .Values.mihomo.image.tag -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" (trimSuffix "/" $registry) (trimPrefix "/" $repository) $tag -}}
{{- else -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.storageClass" -}}
{{- coalesce .Values.persistence.storageClass .Values.global.defaultStorageClass .Values.global.storageClass -}}
{{- end -}}

{{- define "sub2api.serviceFQDN" -}}
{{- printf "%s.%s.svc.cluster.local" .name .namespace -}}
{{- end -}}

{{- define "sub2api.postgresql.fullname" -}}
{{- printf "%s-postgresql" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sub2api.postgresql.secretName" -}}
{{- if .Values.postgresql.connection.secretName -}}
{{- .Values.postgresql.connection.secretName -}}
{{- else if .Values.postgresql.auth.existingSecret -}}
{{- .Values.postgresql.auth.existingSecret -}}
{{- else if and (not .Values.postgresql.enabled) .Values.secrets.postgresql.existingSecret -}}
{{- .Values.secrets.postgresql.existingSecret -}}
{{- else -}}
{{- printf "%s-postgresql" (include "sub2api.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.postgresql.passwordKey" -}}
{{- if .Values.postgresql.connection.passwordKey -}}
{{- .Values.postgresql.connection.passwordKey -}}
{{- else if and (not .Values.postgresql.enabled) .Values.secrets.postgresql.existingSecret (not .Values.postgresql.auth.existingSecret) -}}
postgres-password
{{- else -}}
{{- default "password" .Values.postgresql.auth.secretKeys.userPasswordKey -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.postgresql.host" -}}
{{- if .Values.postgresql.connection.host -}}
{{- .Values.postgresql.connection.host -}}
{{- else if not .Values.postgresql.enabled -}}
{{- .Values.externalPostgresql.host -}}
{{- else if eq (default "standalone" .Values.postgresql.architecture) "replication" -}}
{{- include "sub2api.serviceFQDN" (dict "name" (printf "%s-primary" (include "sub2api.postgresql.fullname" .)) "namespace" .Release.Namespace) -}}
{{- else -}}
{{- include "sub2api.serviceFQDN" (dict "name" (include "sub2api.postgresql.fullname" .) "namespace" .Release.Namespace) -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.postgresql.port" -}}
{{- if .Values.postgresql.connection.port -}}
{{- .Values.postgresql.connection.port -}}
{{- else if not .Values.postgresql.enabled -}}
{{- .Values.externalPostgresql.port -}}
{{- else -}}
{{- default 5432 .Values.postgresql.primary.service.ports.postgresql -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.redis.fullname" -}}
{{- printf "%s-redis" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sub2api.redis.secretName" -}}
{{- if .Values.redis.connection.secretName -}}
{{- .Values.redis.connection.secretName -}}
{{- else if .Values.redis.auth.existingSecret -}}
{{- .Values.redis.auth.existingSecret -}}
{{- else if and (not .Values.redis.enabled) .Values.secrets.redis.existingSecret -}}
{{- .Values.secrets.redis.existingSecret -}}
{{- else -}}
{{- printf "%s-redis" (include "sub2api.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.redis.passwordKey" -}}
{{- if .Values.redis.connection.passwordKey -}}
{{- .Values.redis.connection.passwordKey -}}
{{- else -}}
{{- default "redis-password" .Values.redis.auth.existingSecretPasswordKey -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.redis.host" -}}
{{- if .Values.redis.connection.host -}}
{{- .Values.redis.connection.host -}}
{{- else if not .Values.redis.enabled -}}
{{- .Values.externalRedis.host -}}
{{- else if .Values.redis.sentinel.enabled -}}
{{- if .Values.redis.sentinel.masterService.enabled -}}
{{- include "sub2api.serviceFQDN" (dict "name" (printf "%s-master" (include "sub2api.redis.fullname" .)) "namespace" .Release.Namespace) -}}
{{- else -}}
{{- include "sub2api.serviceFQDN" (dict "name" (include "sub2api.redis.fullname" .) "namespace" .Release.Namespace) -}}
{{- end -}}
{{- else -}}
{{- include "sub2api.serviceFQDN" (dict "name" (printf "%s-master" (include "sub2api.redis.fullname" .)) "namespace" .Release.Namespace) -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.redis.port" -}}
{{- if .Values.redis.connection.port -}}
{{- .Values.redis.connection.port -}}
{{- else if not .Values.redis.enabled -}}
{{- .Values.externalRedis.port -}}
{{- else -}}
{{- default 6379 .Values.redis.master.service.ports.redis -}}
{{- end -}}
{{- end -}}

{{/*
Validate user-facing chart values that would otherwise be silently ignored by
Bitnami dependency charts.
*/}}
{{- define "sub2api.validateSecretName" -}}
{{- $name := default "" .name -}}
{{- if and $name (ne $name (trim $name)) -}}
{{- fail (printf "%s must not contain leading or trailing whitespace" .path) -}}
{{- end -}}
{{- end -}}

{{- define "sub2api.validateValues" -}}
{{- include "sub2api.validateSecretName" (dict "path" "secrets.app.existingSecret" "name" .Values.secrets.app.existingSecret) -}}
{{- include "sub2api.validateSecretName" (dict "path" "secrets.postgresql.existingSecret" "name" .Values.secrets.postgresql.existingSecret) -}}
{{- include "sub2api.validateSecretName" (dict "path" "secrets.redis.existingSecret" "name" .Values.secrets.redis.existingSecret) -}}
{{- include "sub2api.validateSecretName" (dict "path" "postgresql.auth.existingSecret" "name" .Values.postgresql.auth.existingSecret) -}}
{{- include "sub2api.validateSecretName" (dict "path" "postgresql.connection.secretName" "name" .Values.postgresql.connection.secretName) -}}
{{- include "sub2api.validateSecretName" (dict "path" "redis.auth.existingSecret" "name" .Values.redis.auth.existingSecret) -}}
{{- include "sub2api.validateSecretName" (dict "path" "redis.connection.secretName" "name" .Values.redis.connection.secretName) -}}
{{- include "sub2api.validateSecretName" (dict "path" "mihomo.config.existingSecret" "name" .Values.mihomo.config.existingSecret) -}}
{{- if and .Values.postgresql.enabled .Values.secrets.postgresql.existingSecret -}}
{{- fail "secrets.postgresql.existingSecret is only used when postgresql.enabled=false. For bundled Bitnami PostgreSQL, set postgresql.auth.existingSecret instead." -}}
{{- end -}}
{{- if and .Values.redis.enabled .Values.secrets.redis.existingSecret -}}
{{- fail "secrets.redis.existingSecret is only used when redis.enabled=false. For bundled Bitnami Redis, set redis.auth.existingSecret instead." -}}
{{- end -}}
{{- if and .Values.mihomo.enabled (not .Values.mihomo.config.existingSecret) -}}
{{- fail "mihomo.config.existingSecret is required when mihomo.enabled=true" -}}
{{- end -}}
{{- if and .Values.mihomo.enabled (not .Values.mihomo.config.key) -}}
{{- fail "mihomo.config.key is required when mihomo.enabled=true" -}}
{{- end -}}
{{- end -}}

{{/*
Return a base64-encoded Secret data value. When upgrading an existing release,
reuse the existing key unless the user provided a new explicit value.
*/}}
{{- define "sub2api.secretData" -}}
{{- $root := .root -}}
{{- $secretName := .secretName -}}
{{- $key := .key -}}
{{- $value := default "" .value -}}
{{- $generated := default "" .generated -}}
{{- $existing := lookup "v1" "Secret" $root.Release.Namespace $secretName -}}
{{- if ne $value "" -}}
{{- $value | b64enc | quote -}}
{{- else if and $existing (hasKey $existing.data $key) -}}
{{- index $existing.data $key | quote -}}
{{- else -}}
{{- $generated | b64enc | quote -}}
{{- end -}}
{{- end -}}

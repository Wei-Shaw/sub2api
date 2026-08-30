[CmdletBinding()]
param(
    [string]$ApiBase,
    [System.Security.SecureString]$AdminJwt,
    [System.Security.SecureString]$TokenKey,
    [System.Security.SecureString]$PerRequestKey,
    [System.Security.SecureString]$ImageKey,
    [ValidateRange(1, 1000)]
    [decimal]$Markup = 1.20,
    [string]$PricingUrl = 'https://api.x5m5x.com/pricing/',
    [string]$UpstreamApiBase = 'https://api.x5m5x.com/v1',
    [string]$FixtureDirectory,
    [ValidateRange(1, 10000)]
    [int]$AccountConcurrency = 3,
    [ValidateRange(0, 1000000)]
    [int]$AccountPriority = 50,
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$')]
    [string]$NamePrefix = 'x5m5x',
    [switch]$PublishGroups,
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:AdminApiRoot = $null
$script:AdminBearerToken = $null
$script:MarkupValue = $Markup
$script:UpstreamApiRoot = $null

function ConvertFrom-ProtectedString {
    param(
        [Parameter(Mandatory)]
        [System.Security.SecureString]$Value
    )

    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Value)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
    }
    finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
    }
}

function Get-ProtectedValue {
    param(
        [System.Security.SecureString]$Provided,
        [Parameter(Mandatory)]
        [string]$EnvironmentName,
        [Parameter(Mandatory)]
        [string]$Prompt
    )

    if ($null -ne $Provided -and $Provided.Length -gt 0) {
        return $Provided
    }

    $environmentValue = [Environment]::GetEnvironmentVariable($EnvironmentName)
    if (-not [string]::IsNullOrWhiteSpace($environmentValue)) {
        try {
            return ConvertTo-SecureString -String $environmentValue -AsPlainText -Force
        }
        finally {
            $environmentValue = $null
        }
    }

    $value = Read-Host -Prompt $Prompt -AsSecureString
    if ($null -eq $value -or $value.Length -eq 0) {
        throw "$Prompt cannot be empty."
    }
    return $value
}

function Resolve-AdminApiRoot {
    param([string]$Value)

    $candidate = $Value
    if ([string]::IsNullOrWhiteSpace($candidate)) {
        $candidate = [Environment]::GetEnvironmentVariable('SUB2API_ADMIN_API_BASE')
    }
    if ([string]::IsNullOrWhiteSpace($candidate)) {
        $candidate = Read-Host -Prompt 'Sub2API API base [http://127.0.0.1:8080]'
    }
    if ([string]::IsNullOrWhiteSpace($candidate)) {
        $candidate = 'http://127.0.0.1:8080'
    }

    $candidate = $candidate.Trim().TrimEnd('/')
    $uri = $null
    if (-not [Uri]::TryCreate($candidate, [UriKind]::Absolute, [ref]$uri)) {
        throw 'SUB2API_ADMIN_API_BASE must be an absolute HTTP(S) URL.'
    }
    if ($uri.Scheme -notin @('http', 'https')) {
        throw 'SUB2API_ADMIN_API_BASE must use HTTP or HTTPS.'
    }
    $isLoopback = $uri.IsLoopback -or $uri.Host -in @('localhost', '127.0.0.1', '::1')
    if ($uri.Scheme -ne 'https' -and -not $isLoopback) {
        throw 'Refusing to send an administrator JWT over non-HTTPS to a non-loopback host.'
    }

    if ($candidate -match '/api/v1$') {
        return $candidate
    }
    return "$candidate/api/v1"
}

function Resolve-UpstreamApiRoot {
    param([Parameter(Mandatory)][string]$Value)

    $candidate = $Value.Trim().TrimEnd('/')
    $uri = $null
    if (-not [Uri]::TryCreate($candidate, [UriKind]::Absolute, [ref]$uri) -or $uri.Scheme -ne 'https') {
        throw 'UpstreamApiBase must be an absolute HTTPS URL.'
    }
    return $candidate
}

function Resolve-PricingSourceUri {
    param([Parameter(Mandatory)][string]$Value)

    $candidate = $Value.Trim()
    $uri = $null
    if (-not [Uri]::TryCreate($candidate, [UriKind]::Absolute, [ref]$uri) -or $uri.Scheme -ne 'https') {
        throw 'PricingUrl must be an absolute HTTPS URL.'
    }
    return $candidate
}

function Get-FixturePath {
    param(
        [Parameter(Mandatory)][string]$Directory,
        [Parameter(Mandatory)][string]$FileName
    )

    $resolvedDirectory = (Resolve-Path -LiteralPath $Directory -ErrorAction Stop).Path
    $candidate = Join-Path -Path $resolvedDirectory -ChildPath $FileName
    if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) {
        throw "Fixture file '$FileName' was not found in FixtureDirectory."
    }
    return $candidate
}

function Get-PricingHtml {
    param(
        [Parameter(Mandatory)][string]$LiveUrl,
        [string]$FixtureDir
    )

    if (-not [string]::IsNullOrWhiteSpace($FixtureDir)) {
        $path = Get-FixturePath -Directory $FixtureDir -FileName 'pricing.html'
        $content = Get-Content -Raw -LiteralPath $path -Encoding UTF8
        if ([string]::IsNullOrWhiteSpace($content)) {
            throw 'The pricing fixture is empty.'
        }
        Write-Host "Loaded upstream pricing from offline fixture 'pricing.html'."
        return $content
    }

    $uri = Resolve-PricingSourceUri -Value $LiveUrl
    $response = $null
    for ($attempt = 1; $attempt -le 5; $attempt++) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Method Get -Uri $uri -Headers @{ Accept = 'text/html' } -HttpVersion 1.1 -TimeoutSec 45
            break
        }
        catch {
            $statusCode = $null
            try { $statusCode = [int]$_.Exception.Response.StatusCode } catch { $statusCode = $null }
            if ($attempt -lt 5) {
                Write-Host "Upstream pricing download attempt $attempt failed; retrying."
                Start-Sleep -Seconds (2 * $attempt)
                continue
            }
            if ($null -ne $statusCode -and $statusCode -gt 0) {
                throw "Upstream pricing download failed after 5 attempts (HTTP $statusCode)."
            }
            throw 'Upstream pricing download failed after 5 attempts.'
        }
    }
    if ([string]::IsNullOrWhiteSpace($response.Content)) {
        throw 'Upstream pricing page returned empty content.'
    }
    Write-Host "Downloaded current upstream pricing from $uri."
    return [string]$response.Content
}

function ConvertFrom-HtmlText {
    param([AllowEmptyString()][string]$Value)

    if ($null -eq $Value) { return '' }
    $withoutTags = [regex]::Replace($Value, '(?is)<[^>]+>', '')
    return [Net.WebUtility]::HtmlDecode($withoutTags).Trim()
}

function ConvertFrom-UpstreamPriceText {
    param(
        [AllowEmptyString()][string]$Value,
        [switch]$AllowMissing
    )

    $text = ConvertFrom-HtmlText -Value $Value
    $text = $text.Replace('¥', '').Replace('￥', '').Replace(',', '').Trim()
    if ($text -in @('', '-', '—', '未核实', '暂无')) {
        if ($AllowMissing) { return $null }
        throw 'A required upstream price is missing.'
    }
    if ($text -notmatch '^\d+(?:\.\d+)?$') {
        throw "Unrecognized upstream price value '$text'."
    }
    return [decimal]::Parse($text, [Globalization.CultureInfo]::InvariantCulture)
}

function Get-AttributeValue {
    param(
        [Parameter(Mandatory)][string]$Html,
        [Parameter(Mandatory)][string]$Name
    )

    $match = [regex]::Match($Html, ('(?is)\b' + [regex]::Escape($Name) + '\s*=\s*["'']([^"'']+)["'']'))
    if (-not $match.Success) { return $null }
    return [Net.WebUtility]::HtmlDecode($match.Groups[1].Value).Trim()
}

function Get-CellHtmlByLabel {
    param(
        [Parameter(Mandatory)][string]$RowHtml,
        [Parameter(Mandatory)][string]$Label
    )

    $pattern = '(?is)<td\b(?=[^>]*\bdata-label\s*=\s*["'']' + [regex]::Escape($Label) + '["''])[^>]*>(.*?)</td>'
    $match = [regex]::Match($RowHtml, $pattern)
    if (-not $match.Success) {
        throw "Upstream token row is missing the '$Label' cell."
    }
    return $match.Groups[1].Value
}

function Get-StrongPrices {
    param([Parameter(Mandatory)][string]$Html)

    $values = @()
    foreach ($match in [regex]::Matches($Html, '(?is)<strong\b[^>]*>(.*?)</strong>')) {
        $values += ,(ConvertFrom-UpstreamPriceText -Value $match.Groups[1].Value -AllowMissing)
    }
    return $values
}

function Assert-UniqueModels {
    param(
        [Parameter(Mandatory)][object[]]$Models,
        [Parameter(Mandatory)][string]$Label
    )

    $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($model in $Models) {
        $name = [string]$model.Name
        if ([string]::IsNullOrWhiteSpace($name)) {
            throw "$Label contains an empty model name."
        }
        if (-not $seen.Add($name)) {
            throw "$Label contains a case-insensitive duplicate model '$name'."
        }
    }
}

function ConvertFrom-UpstreamPricingHtml {
    param([Parameter(Mandatory)][string]$Html)

    $tokenModels = @()
    $tokenRows = [regex]::Matches($Html, '(?is)<tr\b(?=[^>]*\bclass\s*=\s*["''][^"'']*\btoken-model\b[^"'']*["''])[^>]*>.*?</tr>')
    foreach ($rowMatch in $tokenRows) {
        $row = $rowMatch.Value
        $name = Get-AttributeValue -Html $row -Name 'data-model'
        if ([string]::IsNullOrWhiteSpace($name)) {
            throw 'An upstream token row has no data-model attribute.'
        }
        $inputValues = @(Get-StrongPrices -Html (Get-CellHtmlByLabel -RowHtml $row -Label '输入'))
        $outputValues = @(Get-StrongPrices -Html (Get-CellHtmlByLabel -RowHtml $row -Label '输出'))
        $cacheValues = @(Get-StrongPrices -Html (Get-CellHtmlByLabel -RowHtml $row -Label '缓存'))
        if ($inputValues.Count -ne 1 -or $outputValues.Count -ne 1 -or $cacheValues.Count -ne 2) {
            throw "Upstream token price row '$name' has an unexpected layout."
        }
        if ($null -eq $inputValues[0] -or $null -eq $outputValues[0]) {
            throw "Upstream token price row '$name' is missing input or output pricing."
        }
        $tokenModels += [pscustomobject]@{
            Name = $name
            Input = [decimal]$inputValues[0]
            Output = [decimal]$outputValues[0]
            CacheRead = $cacheValues[0]
            CacheWrite = $cacheValues[1]
        }
    }

    $perRequestModels = @()
    $requestRows = [regex]::Matches($Html, '(?is)<tr\b(?=[^>]*\bclass\s*=\s*["''][^"'']*\brequest-table-row\b[^"'']*["''])[^>]*>.*?</tr>')
    foreach ($rowMatch in $requestRows) {
        $row = $rowMatch.Value
        $nameMatch = [regex]::Match($row, '(?is)<th\b(?=[^>]*\bclass\s*=\s*["''][^"'']*\bm-name\b[^"'']*["''])[^>]*>(.*?)</th>')
        if (-not $nameMatch.Success) {
            throw 'An upstream per-request row has no model name.'
        }
        $name = ConvertFrom-HtmlText -Value $nameMatch.Groups[1].Value
        $prices = @()
        foreach ($priceMatch in [regex]::Matches($row, '(?:¥|￥)\s*\d+(?:\.\d+)?')) {
            $prices += ,(ConvertFrom-UpstreamPriceText -Value $priceMatch.Value)
        }
        if ($prices.Count -ne 3) {
            throw "Upstream per-request row '$name' must expose exactly three context-tier prices."
        }
        $perRequestModels += [pscustomobject]@{
            Name = $name
            Small = [decimal]$prices[0]
            Middle = [decimal]$prices[1]
            Large = [decimal]$prices[2]
        }
    }

    $imageModels = @()
    $imageSection = [regex]::Match($Html, '(?is)<section\b(?=[^>]*\bid\s*=\s*["'']sec-img["''])[^>]*>(.*?)</section>')
    if ($imageSection.Success) {
        foreach ($row in [regex]::Matches($imageSection.Groups[1].Value, '(?is)<div\b(?=[^>]*\bclass\s*=\s*["''][^"'']*\bm-row\b[^"'']*["''])[^>]*>(.*?)</div>\s*</div>')) {
            $nameMatch = [regex]::Match($row.Value, '(?is)<span\b(?=[^>]*\bclass\s*=\s*["''][^"'']*\bm-name\b[^"'']*["''])[^>]*>(.*?)</span>')
            $priceMatch = [regex]::Match($row.Value, '(?is)<span\b(?=[^>]*\bclass\s*=\s*["''][^"'']*\bm-price\b[^"'']*["''])[^>]*>([^<]+)')
            if (-not $nameMatch.Success -or -not $priceMatch.Success) {
                throw 'An upstream image row has an unexpected layout.'
            }
            $imageModels += [pscustomobject]@{
                Name = ConvertFrom-HtmlText -Value $nameMatch.Groups[1].Value
                PerImage = ConvertFrom-UpstreamPriceText -Value $priceMatch.Groups[1].Value
            }
        }
    }

    Assert-UniqueModels -Models $tokenModels -Label 'Upstream token pricing'
    Assert-UniqueModels -Models $perRequestModels -Label 'Upstream per-request pricing'
    Assert-UniqueModels -Models $imageModels -Label 'Upstream image pricing'
    if ($tokenModels.Count -ne 33 -or $perRequestModels.Count -ne 14 -or $imageModels.Count -ne 1) {
        throw "Upstream pricing catalogue count mismatch: expected 33/14/1, got $($tokenModels.Count)/$($perRequestModels.Count)/$($imageModels.Count). Refusing to provision incomplete pricing."
    }
    $plainPage = ConvertFrom-HtmlText -Value $Html
    if ($plainPage -notmatch '(?is)DeepSeek.*?09:00\s*[–—-]\s*12:00.*?14:00\s*[–—-]\s*18:00.*?(?:×|x)\s*2') {
        throw 'The expected DeepSeek weekday peak-price notice (09:00-12:00, 14:00-18:00, x2) is missing or changed; refusing to install a stale time-pricing rule.'
    }

    return [pscustomobject]@{
        Token = $tokenModels
        PerRequest = $perRequestModels
        Image = $imageModels
    }
}

function ConvertFrom-ModelsResponse {
    param(
        [Parameter(Mandatory)]$Response,
        [Parameter(Mandatory)][string]$Label
    )

    $items = if ($null -ne $Response.PSObject.Properties['data']) { @($Response.data) } else { @($Response) }
    $models = @()
    $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($item in $items) {
        $id = if ($item -is [string]) { [string]$item } elseif ($null -ne $item.PSObject.Properties['id']) { [string]$item.id } else { '' }
        $id = $id.Trim()
        if ([string]::IsNullOrWhiteSpace($id)) {
            throw "$Label /models response contains an entry without an id."
        }
        if (-not $seen.Add($id)) {
            throw "$Label /models response contains a case-insensitive duplicate id '$id'."
        }
        $models += $id
    }
    return $models
}

function Get-UpstreamModels {
    param(
        [Parameter(Mandatory)][string]$Label,
        [System.Security.SecureString]$Key,
        [string]$FixturePath
    )

    if (-not [string]::IsNullOrWhiteSpace($FixturePath)) {
        $response = Get-Content -Raw -LiteralPath $FixturePath -Encoding UTF8 | ConvertFrom-Json
        $models = @(ConvertFrom-ModelsResponse -Response $response -Label $Label)
        Write-Host "Loaded $($models.Count) $Label models from offline fixture."
        return $models
    }
    if ($null -eq $Key -or $Key.Length -eq 0) {
        throw "A key is required to query the $Label upstream model catalogue."
    }

    $plainKey = ConvertFrom-ProtectedString -Value $Key
    try {
        $response = $null
        for ($attempt = 1; $attempt -le 5; $attempt++) {
            try {
                $response = Invoke-RestMethod -Method Get -Uri "$script:UpstreamApiRoot/models" -Headers @{
                    Authorization = "Bearer $plainKey"
                    Accept = 'application/json'
                } -HttpVersion 1.1 -TimeoutSec 45
                break
            }
            catch {
                $statusCode = $null
                try { $statusCode = [int]$_.Exception.Response.StatusCode } catch { $statusCode = $null }
                if ($attempt -lt 5) {
                    Write-Host "$Label upstream /models attempt $attempt failed; retrying without exposing credentials."
                    Start-Sleep -Seconds (2 * $attempt)
                    continue
                }
                if ($null -ne $statusCode -and $statusCode -gt 0) {
                    throw "$Label upstream /models verification failed after 5 attempts (HTTP $statusCode)."
                }
                throw "$Label upstream /models verification failed after 5 attempts."
            }
        }
        $models = @(ConvertFrom-ModelsResponse -Response $response -Label $Label)
        Write-Host "Verified $($models.Count) $Label models with the live upstream key."
        return $models
    }
    finally {
        $plainKey = $null
        $response = $null
    }
}

function Assert-ModelSetMatchesPricing {
    param(
        [Parameter(Mandatory)][string[]]$Actual,
        [Parameter(Mandatory)][object[]]$PricingModels,
        [Parameter(Mandatory)][int]$ExpectedCount,
        [Parameter(Mandatory)][string]$Label
    )

    $actualByName = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($name in $Actual) {
        if (-not $actualByName.TryAdd($name, $name)) {
            throw "$Label /models response contains a case-insensitive duplicate id '$name'."
        }
    }
    $pricingByName = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($model in $PricingModels) {
        $name = [string]$model.Name
        if (-not $pricingByName.TryAdd($name, $name)) {
            throw "$Label pricing contains a case-insensitive duplicate model '$name'."
        }
    }

    $missing = @($pricingByName.Values | Where-Object { -not $actualByName.ContainsKey($_) } | Sort-Object)
    $unexpected = @($actualByName.Values | Where-Object { -not $pricingByName.ContainsKey($_) } | Sort-Object)
    if ($Actual.Count -ne $ExpectedCount -or $actualByName.Count -ne $ExpectedCount -or $pricingByName.Count -ne $ExpectedCount -or $missing.Count -gt 0 -or $unexpected.Count -gt 0) {
        $missingText = if ($missing.Count -eq 0) { 'none' } else { $missing -join ', ' }
        $unexpectedText = if ($unexpected.Count -eq 0) { 'none' } else { $unexpected -join ', ' }
        throw "$Label model verification failed: expected exactly $ExpectedCount models; received $($Actual.Count); missing: $missingText; unexpected: $unexpectedText."
    }

    # Pricing HTML sometimes changes only the casing of a model id. The
    # authenticated /models response is the canonical request spelling, so
    # normalize the price entry before groups, mappings, and channels are built.
    foreach ($model in $PricingModels) {
        $model.Name = $actualByName[[string]$model.Name]
    }
}

function Invoke-AdminApi {
    param(
        [Parameter(Mandatory)]
        [ValidateSet('GET', 'POST', 'PUT')]
        [string]$Method,
        [Parameter(Mandatory)]
        [string]$Path,
        [object]$Body,
        [string]$Operation = 'request'
    )

    $headers = @{
        Authorization = "Bearer $script:AdminBearerToken"
        Accept = 'application/json'
    }
    if ($Method -ne 'GET') {
        # A fresh key is used for each logical write. If Invoke-RestMethod is retried
        # manually, rerun the provisioning tool: name-based upserts are authoritative.
        $headers['Idempotency-Key'] = "provision-x5m5x-$([Guid]::NewGuid().ToString('N'))"
    }

    $uri = "$script:AdminApiRoot$Path"
    try {
        if ($Method -eq 'GET') {
            $response = Invoke-RestMethod -Method Get -Uri $uri -Headers $headers
        }
        else {
            $json = $Body | ConvertTo-Json -Depth 30 -Compress
            try {
                $response = Invoke-RestMethod -Method $Method -Uri $uri -Headers $headers -ContentType 'application/json' -Body $json
            }
            finally {
                $json = $null
            }
        }
    }
    catch {
        $statusCode = $null
        try {
            $statusCode = [int]$_.Exception.Response.StatusCode
        }
        catch {
            $statusCode = $null
        }
        if ($null -ne $statusCode) {
            throw "Admin API $Operation failed (HTTP $statusCode, $Method $Path)."
        }
        throw "Admin API $Operation failed ($Method $Path)."
    }

    if ($null -eq $response -or $response.code -ne 0) {
        throw "Admin API $Operation returned an unsuccessful response ($Method $Path)."
    }
    return $response.data
}

function Get-ExactNamedResource {
    param(
        [Parameter(Mandatory)]
        [ValidateSet('groups', 'accounts', 'channels')]
        [string]$Resource,
        [Parameter(Mandatory)]
        [string]$Name
    )

    $encoded = [Uri]::EscapeDataString($Name)
    $data = Invoke-AdminApi -Method GET -Path "/admin/${Resource}?page=1&page_size=1000&search=$encoded" -Operation "list $Resource"
    $matches = @($data.items | Where-Object { $_.name -ceq $Name })
    if ($matches.Count -gt 1) {
        throw "Multiple $Resource named '$Name' exist; refusing to choose one."
    }
    if ($matches.Count -eq 1) {
        return $matches[0]
    }
    return $null
}

function Convert-UpstreamPerMillionToUsdPerToken {
    param([Parameter(Mandatory)][decimal]$Value)
    # Despite the currency glyph on the HTML page, the upstream usage ledger
    # debits the same numeric value with unit=USD. Do not apply FX conversion.
    return [Math]::Round(($Value * $script:MarkupValue / [decimal]1000000), 15, [MidpointRounding]::AwayFromZero)
}

function Convert-UpstreamPerRequestToUsd {
    param([Parameter(Mandatory)][decimal]$Value)
    return [Math]::Round(($Value * $script:MarkupValue), 12, [MidpointRounding]::AwayFromZero)
}

function Convert-OptionalTokenPrice {
    param($Value)
    if ($null -eq $Value) {
        return $null
    }
    return Convert-UpstreamPerMillionToUsdPerToken -Value ([decimal]$Value)
}

function New-IdentityMapping {
    param([Parameter(Mandatory)][string[]]$Models)
    $mapping = [ordered]@{}
    foreach ($model in $Models) {
        $mapping[$model] = $model
    }
    return $mapping
}

function New-TokenPricingEntries {
    param([Parameter(Mandatory)][object[]]$Models)
    $entries = @()
    foreach ($model in $Models) {
        $entries += [ordered]@{
            platform = 'openai'
            models = @([string]$model.Name)
            billing_mode = 'token'
            input_price = Convert-UpstreamPerMillionToUsdPerToken -Value ([decimal]$model.Input)
            output_price = Convert-UpstreamPerMillionToUsdPerToken -Value ([decimal]$model.Output)
            cache_write_price = Convert-OptionalTokenPrice -Value $model.CacheWrite
            cache_read_price = Convert-OptionalTokenPrice -Value $model.CacheRead
            intervals = @()
        }
        if ([string]$model.Name -like 'deepseek-*') {
            # The upstream doubles all DeepSeek token charges on workdays during
            # these Asia/Shanghai peak windows. Reproduce that upstream rule on
            # top of the already-marked-up normal price so peak requests remain
            # profitable as well.
            $entries[-1]['time_pricing'] = [ordered]@{
                timezone = 'Asia/Shanghai'
                weekdays_only = $true
                periods = @(
                    [ordered]@{ start_time = '09:00'; end_time = '12:00'; multiplier = 2.0 },
                    [ordered]@{ start_time = '14:00'; end_time = '18:00'; multiplier = 2.0 }
                )
            }
        }
    }
    return $entries
}

function New-PerRequestPricingEntries {
    param([Parameter(Mandatory)][object[]]$Models)
    $entries = @()
    foreach ($model in $Models) {
        $base = Convert-UpstreamPerRequestToUsd -Value ([decimal]$model.Small)
        $middle = Convert-UpstreamPerRequestToUsd -Value ([decimal]$model.Middle)
        $large = Convert-UpstreamPerRequestToUsd -Value ([decimal]$model.Large)
        $entries += [ordered]@{
            platform = 'openai'
            models = @([string]$model.Name)
            billing_mode = 'per_request'
            per_request_price = $base
            intervals = @(
                [ordered]@{ min_tokens = 0; max_tokens = 256000; tier_label = '<=256K'; per_request_price = $base; sort_order = 0 },
                [ordered]@{ min_tokens = 256000; max_tokens = 512000; tier_label = '256K-512K'; per_request_price = $middle; sort_order = 1 },
                [ordered]@{ min_tokens = 512000; max_tokens = $null; tier_label = '>512K'; per_request_price = $large; sort_order = 2 }
            )
        }
    }
    return $entries
}

function New-ImagePricingEntries {
    param([Parameter(Mandatory)][object[]]$Models)
    $entries = @()
    foreach ($model in $Models) {
        $entries += [ordered]@{
            platform = 'openai'
            models = @([string]$model.Name)
            billing_mode = 'image'
            per_request_price = Convert-UpstreamPerRequestToUsd -Value ([decimal]$model.PerImage)
            intervals = @()
        }
    }
    return $entries
}

function Ensure-Group {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Description,
        [Parameter(Mandatory)][decimal]$RateMultiplier,
        [Parameter(Mandatory)][bool]$AllowImages,
        [Parameter(Mandatory)][string[]]$Models
    )

    $existing = Get-ExactNamedResource -Resource groups -Name $Name
    $stagedExclusive = $true
    if ($null -ne $existing) {
        # A default (non-publishing) run closes an accidentally public managed
        # group before touching its pricing/accounts. An explicit publishing run
        # keeps an already-public group available while refreshing it.
        $stagedExclusive = if ($PublishGroups) { [bool]$existing.is_exclusive } else { $true }
    }
    $body = [ordered]@{
        name = $Name
        description = $Description
        platform = 'openai'
        rate_multiplier = $RateMultiplier
        is_exclusive = $stagedExclusive
        status = 'active'
        subscription_type = 'standard'
        long_context_pricing_enabled = $true
        model_pricing = @()
        allow_image_generation = $AllowImages
        allow_batch_image_generation = $false
        image_rate_independent = $false
        image_price_1k = -1
        image_price_2k = -1
        image_price_4k = -1
        models_list_config = [ordered]@{ enabled = $true; models = $Models }
    }

    if ($DryRun) {
        $verb = if ($null -eq $existing) { 'create' } else { 'update' }
        Write-Host "[DRY-RUN] $verb group '$Name' (staged/private until completion)"
        if ($null -ne $existing) { return $existing }
        return [pscustomobject]@{ id = 0; name = $Name; is_exclusive = $true }
    }

    if ($null -eq $existing) {
        # New managed groups always start private. Publishing happens only after
        # channels and accounts have been fully provisioned.
        $body.is_exclusive = $true
        $created = Invoke-AdminApi -Method POST -Path '/admin/groups' -Body $body -Operation "create group $Name"
        Write-Host "Created group '$Name' (id=$($created.id), exclusive=true)."
        return $created
    }

    $updated = Invoke-AdminApi -Method PUT -Path "/admin/groups/$($existing.id)" -Body $body -Operation "update group $Name"
    Write-Host "Updated group '$Name' (id=$($updated.id))."
    return $updated
}

function Set-GroupPublication {
    param(
        [Parameter(Mandatory)][object]$Group,
        [Parameter(Mandatory)][bool]$Exclusive
    )

    if ($DryRun) {
        Write-Host "[DRY-RUN] set group '$($Group.name)' exclusive=$($Exclusive.ToString().ToLowerInvariant())"
        return
    }

    $body = [ordered]@{ is_exclusive = $Exclusive }
    $null = Invoke-AdminApi -Method PUT -Path "/admin/groups/$($Group.id)" -Body $body -Operation "publish group $($Group.name)"
    Write-Host "Group '$($Group.name)' exclusive=$($Exclusive.ToString().ToLowerInvariant())."
}

function Ensure-Channel {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][long]$GroupId,
        [Parameter(Mandatory)][object[]]$Pricing,
        [Parameter(Mandatory)][System.Collections.IDictionary]$IdentityMapping
    )

    $existing = Get-ExactNamedResource -Resource channels -Name $Name
    $body = [ordered]@{
        name = $Name
        description = 'Managed x5m5x real-billing channel; separate from display pricing.'
        status = 'active'
        group_ids = @($GroupId)
        model_pricing = $Pricing
        model_mapping = [ordered]@{ openai = $IdentityMapping }
        billing_model_source = 'requested'
        restrict_models = $true
        features = '[]'
        features_config = @{}
        apply_pricing_to_account_stats = $false
        account_stats_pricing_rules = @()
    }

    if ($DryRun) {
        $verb = if ($null -eq $existing) { 'create' } else { 'update' }
        Write-Host "[DRY-RUN] $verb channel '$Name' with $($Pricing.Count) model price entries"
        return
    }

    if ($null -eq $existing) {
        $created = Invoke-AdminApi -Method POST -Path '/admin/channels' -Body $body -Operation "create channel $Name"
        Write-Host "Created channel '$Name' (id=$($created.id))."
        return
    }

    $updated = Invoke-AdminApi -Method PUT -Path "/admin/channels/$($existing.id)" -Body $body -Operation "update channel $Name"
    Write-Host "Updated channel '$Name' (id=$($updated.id))."
}

function Ensure-Account {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][long]$GroupId,
        [Parameter(Mandatory)][System.Collections.IDictionary]$IdentityMapping,
        [System.Security.SecureString]$Key
    )

    $existing = Get-ExactNamedResource -Resource accounts -Name $Name
    if ($null -ne $existing -and ($existing.platform -ne 'openai' -or $existing.type -ne 'apikey')) {
        throw "Account '$Name' exists but is not an OpenAI API-key account; refusing to overwrite it."
    }

    if ($DryRun) {
        $verb = if ($null -eq $existing) { 'create' } else { 'update' }
        Write-Host "[DRY-RUN] $verb account '$Name' and bind it only to group id=$GroupId"
        return
    }
    if ($null -eq $Key -or $Key.Length -eq 0) {
        throw "A key is required for account '$Name'."
    }

    $plainKey = ConvertFrom-ProtectedString -Value $Key
    try {
        $body = [ordered]@{
            name = $Name
            notes = 'Managed by tools/provision-x5m5x.ps1.'
            platform = 'openai'
            type = 'apikey'
            credentials = [ordered]@{
                api_key = $plainKey
                base_url = $script:UpstreamApiRoot
                model_mapping = $IdentityMapping
            }
            concurrency = $AccountConcurrency
            priority = $AccountPriority
            rate_multiplier = 1.0
            group_ids = @($GroupId)
        }

        if ($null -eq $existing) {
            $body.extra = @{}
            $account = Invoke-AdminApi -Method POST -Path '/admin/accounts' -Body $body -Operation "create account $Name"
            Write-Host "Created account '$Name' (id=$($account.id))."
        }
        else {
            $body.status = 'active'
            $account = Invoke-AdminApi -Method PUT -Path "/admin/accounts/$($existing.id)" -Body $body -Operation "update account $Name"
            Write-Host "Updated account '$Name' (id=$($account.id))."
        }

        $null = Invoke-AdminApi -Method POST -Path "/admin/accounts/$($account.id)/schedulable" -Body @{ schedulable = $true } -Operation "enable account $Name"
    }
    finally {
        $body = $null
        $plainKey = $null
    }
}

try {
    if (-not [string]::IsNullOrWhiteSpace($FixtureDirectory) -and -not $DryRun) {
        throw 'FixtureDirectory is allowed only with DryRun; real provisioning must verify live upstream state.'
    }
    $script:UpstreamApiRoot = Resolve-UpstreamApiRoot -Value $UpstreamApiBase
    $pricingHtml = Get-PricingHtml -LiveUrl $PricingUrl -FixtureDir $FixtureDirectory
    $catalogue = ConvertFrom-UpstreamPricingHtml -Html $pricingHtml
    $pricingHtml = $null
    $tokenModels = @($catalogue.Token)
    $perRequestModels = @($catalogue.PerRequest)
    $imageModels = @($catalogue.Image)
    Write-Host "Validated current upstream pricing catalogue: $($tokenModels.Count) token, $($perRequestModels.Count) per-request, $($imageModels.Count) image."

    if (-not $DryRun) {
        $TokenKey = Get-ProtectedValue -Provided $TokenKey -EnvironmentName 'X5M5X_TOKEN_KEY' -Prompt 'x5m5x token-billing API key'
        $PerRequestKey = Get-ProtectedValue -Provided $PerRequestKey -EnvironmentName 'X5M5X_PER_REQUEST_KEY' -Prompt 'x5m5x per-request API key'
        $ImageKey = Get-ProtectedValue -Provided $ImageKey -EnvironmentName 'X5M5X_IMAGE_KEY' -Prompt 'x5m5x image API key'

        $tokenModelNames = @(Get-UpstreamModels -Label 'token' -Key $TokenKey)
        $perRequestModelNames = @(Get-UpstreamModels -Label 'per-request' -Key $PerRequestKey)
        $imageModelNames = @(Get-UpstreamModels -Label 'image' -Key $ImageKey)
    }
    elseif (-not [string]::IsNullOrWhiteSpace($FixtureDirectory)) {
        $tokenModelNames = @(Get-UpstreamModels -Label 'token' -FixturePath (Get-FixturePath -Directory $FixtureDirectory -FileName 'token-models.json'))
        $perRequestModelNames = @(Get-UpstreamModels -Label 'per-request' -FixturePath (Get-FixturePath -Directory $FixtureDirectory -FileName 'per-request-models.json'))
        $imageModelNames = @(Get-UpstreamModels -Label 'image' -FixturePath (Get-FixturePath -Directory $FixtureDirectory -FileName 'image-models.json'))
    }
    else {
        # A keyless dry run can still validate and price the live HTML catalogue.
        # The non-dry path always replaces these names with authenticated /models.
        $tokenModelNames = @($tokenModels.Name)
        $perRequestModelNames = @($perRequestModels.Name)
        $imageModelNames = @($imageModels.Name)
        Write-Host '[DRY-RUN] Upstream /models verification skipped because no model fixtures or keys are used.'
    }

    Assert-ModelSetMatchesPricing -Actual $tokenModelNames -PricingModels $tokenModels -ExpectedCount 33 -Label 'Token'
    Assert-ModelSetMatchesPricing -Actual $perRequestModelNames -PricingModels $perRequestModels -ExpectedCount 14 -Label 'Per-request'
    Assert-ModelSetMatchesPricing -Actual $imageModelNames -PricingModels $imageModels -ExpectedCount 1 -Label 'Image'

    $tokenMapping = New-IdentityMapping -Models $tokenModelNames
    $perRequestMapping = New-IdentityMapping -Models $perRequestModelNames
    $imageMapping = New-IdentityMapping -Models $imageModelNames
    $tokenPricing = New-TokenPricingEntries -Models $tokenModels
    $perRequestPricing = New-PerRequestPricingEntries -Models $perRequestModels
    $imagePricing = New-ImagePricingEntries -Models $imageModels

    $script:AdminApiRoot = Resolve-AdminApiRoot -Value $ApiBase
    $AdminJwt = Get-ProtectedValue -Provided $AdminJwt -EnvironmentName 'SUB2API_ADMIN_JWT' -Prompt 'Sub2API administrator JWT'
    $script:AdminBearerToken = ConvertFrom-ProtectedString -Value $AdminJwt
    $script:AdminBearerToken = $script:AdminBearerToken.Trim()
    if ($script:AdminBearerToken.StartsWith('Bearer ', [StringComparison]::OrdinalIgnoreCase)) {
        $script:AdminBearerToken = $script:AdminBearerToken.Substring(7).Trim()
    }
    if ([string]::IsNullOrWhiteSpace($script:AdminBearerToken)) {
        throw 'Administrator JWT cannot be empty.'
    }

    # Groups are created first as private resources. Channels are attached before
    # any upstream account becomes schedulable in a managed group.
    $tokenGroup = Ensure-Group -Name '按量分组【成功率百分之99+】' `
        -Description '完全对接官方模型 除上游问题外全天可用 适合企业级用户' `
        -RateMultiplier 1.0 -AllowImages $false -Models $tokenModelNames
    $perRequestGroup = Ensure-Group -Name '按次分组【成功率百分之95+】' `
        -Description '按上下文区间扣费-成功率百分之95+' `
        -RateMultiplier 1.0 -AllowImages $false -Models $perRequestModelNames
    $imageGroup = Ensure-Group -Name '生图分组-按次扣费' `
        -Description '生图稳定版-成功率百分之95' `
        -RateMultiplier 1.0 -AllowImages $true -Models $imageModelNames

    Ensure-Channel -Name "$NamePrefix-payg-channel" -GroupId ([long]$tokenGroup.id) -Pricing $tokenPricing -IdentityMapping $tokenMapping
    Ensure-Channel -Name "$NamePrefix-per-request-channel" -GroupId ([long]$perRequestGroup.id) -Pricing $perRequestPricing -IdentityMapping $perRequestMapping
    Ensure-Channel -Name "$NamePrefix-image-channel" -GroupId ([long]$imageGroup.id) -Pricing $imagePricing -IdentityMapping $imageMapping

    Ensure-Account -Name "$NamePrefix-payg-account" -GroupId ([long]$tokenGroup.id) -IdentityMapping $tokenMapping -Key $TokenKey
    Ensure-Account -Name "$NamePrefix-per-request-account" -GroupId ([long]$perRequestGroup.id) -IdentityMapping $perRequestMapping -Key $PerRequestKey
    Ensure-Account -Name "$NamePrefix-image-account" -GroupId ([long]$imageGroup.id) -IdentityMapping $imageMapping -Key $ImageKey

    $exclusive = -not [bool]$PublishGroups
    Set-GroupPublication -Group $tokenGroup -Exclusive $exclusive
    Set-GroupPublication -Group $perRequestGroup -Exclusive $exclusive
    Set-GroupPublication -Group $imageGroup -Exclusive $exclusive

    if ($DryRun) {
        Write-Host "Dry run complete. No writes were sent. Channel prices use live/fixture upstream numeric prices x$Markup."
    }
    else {
        Write-Host "x5m5x provisioning complete. Channel prices use current upstream numeric prices x$Markup. No secret values were printed."
    }
}
finally {
    $script:AdminBearerToken = $null
    $script:UpstreamApiRoot = $null
    $TokenKey = $null
    $PerRequestKey = $null
    $ImageKey = $null
}

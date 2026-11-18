param(
    [string]$Root = (Split-Path $PSScriptRoot -Parent)
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

$source = Join-Path $Root "docs\images\icon.png"
$icoPath = Join-Path $Root "build\windows\icon.ico"
$appIcon = Join-Path $Root "build\appicon.png"
$extensionIcons = Join-Path $Root "extension\icons"
$frontendPublic = Join-Path $Root "frontend\public"

foreach ($directory in @((Split-Path $source -Parent), $extensionIcons, $frontendPublic)) {
    New-Item -ItemType Directory -Force -Path $directory | Out-Null
}

function New-RoundedPath([float]$x, [float]$y, [float]$width, [float]$height, [float]$radius) {
    $path = New-Object System.Drawing.Drawing2D.GraphicsPath
    $diameter = $radius * 2
    $path.AddArc($x, $y, $diameter, $diameter, 180, 90)
    $path.AddArc($x + $width - $diameter, $y, $diameter, $diameter, 270, 90)
    $path.AddArc($x + $width - $diameter, $y + $height - $diameter, $diameter, $diameter, 0, 90)
    $path.AddArc($x, $y + $height - $diameter, $diameter, $diameter, 90, 90)
    $path.CloseFigure()
    return $path
}

function New-WebEggIcon([int]$size) {
    $bitmap = New-Object System.Drawing.Bitmap $size, $size
    $bitmap.SetResolution(96, 96)
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $graphics.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit
    $graphics.Clear([System.Drawing.Color]::Transparent)

    $scale = $size / 1024.0
    $outer = New-RoundedPath (16 * $scale) (16 * $scale) (992 * $scale) (992 * $scale) (224 * $scale)
    $background = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(255, 11, 11, 11))
    $graphics.FillPath($background, $outer)
    $border = New-Object System.Drawing.Pen ([System.Drawing.Color]::FromArgb(255, 244, 244, 240)), ([Math]::Max(1, 9 * $scale))
    $graphics.DrawPath($border, $outer)

    $egg = New-Object System.Drawing.Drawing2D.GraphicsPath
    $egg.StartFigure()
    $egg.AddBezier(512*$scale, 188*$scale, 662*$scale, 188*$scale, 798*$scale, 370*$scale, 798*$scale, 546*$scale)
    $egg.AddBezier(798*$scale, 546*$scale, 798*$scale, 724*$scale, 672*$scale, 842*$scale, 512*$scale, 842*$scale)
    $egg.AddBezier(512*$scale, 842*$scale, 352*$scale, 842*$scale, 226*$scale, 724*$scale, 226*$scale, 546*$scale)
    $egg.AddBezier(226*$scale, 546*$scale, 226*$scale, 370*$scale, 362*$scale, 188*$scale, 512*$scale, 188*$scale)
    $egg.CloseFigure()
    $white = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(255, 246, 246, 242))
    $graphics.FillPath($white, $egg)

    if ($size -ge 48) {
        $font = New-Object System.Drawing.Font "Consolas", ([Math]::Max(8, 214 * $scale)), ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
        $format = New-Object System.Drawing.StringFormat
        $format.Alignment = [System.Drawing.StringAlignment]::Center
        $format.LineAlignment = [System.Drawing.StringAlignment]::Center
        $black = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(255, 11, 11, 11))
        $rect = New-Object System.Drawing.RectangleF (250*$scale), (360*$scale), (524*$scale), (290*$scale)
        $graphics.DrawString("</>", $font, $black, $rect, $format)
        $black.Dispose()
        $font.Dispose()
        $format.Dispose()
    }

    $white.Dispose()
    $egg.Dispose()
    $border.Dispose()
    $background.Dispose()
    $outer.Dispose()
    $graphics.Dispose()
    return $bitmap
}

$master = New-WebEggIcon 1024
$master.Save($source, [System.Drawing.Imaging.ImageFormat]::Png)
$master.Save($appIcon, [System.Drawing.Imaging.ImageFormat]::Png)
$master.Dispose()

foreach ($size in 16, 32, 48, 128) {
    $icon = New-WebEggIcon $size
    $icon.Save((Join-Path $extensionIcons "icon$size.png"), [System.Drawing.Imaging.ImageFormat]::Png)
    $icon.Dispose()
}

$favicon = New-WebEggIcon 64
$favicon.Save((Join-Path $frontendPublic "favicon.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$favicon.Dispose()

$icoBitmap = New-WebEggIcon 256
$iconHandle = $icoBitmap.GetHicon()
$icon = [System.Drawing.Icon]::FromHandle($iconHandle)
$stream = [System.IO.File]::Open($icoPath, [System.IO.FileMode]::Create)
$icon.Save($stream)
$stream.Dispose()
$icon.Dispose()
$icoBitmap.Dispose()

Write-Output "generated $source"
Write-Output "generated $appIcon"
Write-Output "generated $icoPath"
Write-Output "generated extension icons: $((16,32,48,128 | ForEach-Object { "icon$_.png" }) -join ', ')"
Write-Output "generated favicon: $(Join-Path $frontendPublic 'favicon.png')"

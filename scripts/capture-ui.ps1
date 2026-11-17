param([string]$ProcessName = "WebEgg-v0.7.0", [string]$OutputPath = ".tmp\ui-capture.png")
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing
$source = @"
using System;
using System.Runtime.InteropServices;
public class WinPrint {
    [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr hWnd, IntPtr hdcBlt, int nFlags);
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT lpRect);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
    public struct RECT { public int Left; public int Top; public int Right; public int Bottom; }
}
"@
Add-Type -TypeDefinition $source

$process = Get-Process -Name $ProcessName -ErrorAction Stop | Select-Object -First 1
$process.Refresh()
$handle = $process.MainWindowHandle
if ($handle -eq [IntPtr]::Zero) { throw "main window handle unavailable" }
[void][WinPrint]::ShowWindow($handle, 9)
Start-Sleep -Milliseconds 800

$rect = New-Object WinPrint+RECT
[void][WinPrint]::GetWindowRect($handle, [ref]$rect)
$width = $rect.Right - $rect.Left
$height = $rect.Bottom - $rect.Top
if ($width -lt 400 -or $height -lt 300) { throw "bad window size ${width}x${height}" }

$bitmap = New-Object System.Drawing.Bitmap $width, $height
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$hdc = $graphics.GetHdc()
[void][WinPrint]::PrintWindow($handle, $hdc, 2)
$graphics.ReleaseHdc($hdc)
$fullPath = if ([System.IO.Path]::IsPathRooted($OutputPath)) { $OutputPath } else { Join-Path (Get-Location) $OutputPath }
$directory = Split-Path $fullPath -Parent
if ($directory) { New-Item -ItemType Directory -Force -Path $directory | Out-Null }
$bitmap.Save($fullPath, [System.Drawing.Imaging.ImageFormat]::Png)
$graphics.Dispose()
$bitmap.Dispose()
Write-Output "captured $fullPath ($width x $height)"

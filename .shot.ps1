param([string]$Out = 'C:\DEV\hyphp\.shot.png')

Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;
using System.Runtime.InteropServices;
public class W {
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
  [StructLayout(LayoutKind.Sequential)] public struct R { public int L, T, Rt, B; }
}
'@

$p = Get-Process hyphp-dbg -ErrorAction Stop | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if (-not $p) { throw 'janela nao encontrada' }
$h = $p.MainWindowHandle
[W+R]$r = New-Object W+R
[void][W]::GetWindowRect($h, [ref]$r)
$w = $r.Rt - $r.L; $ht = $r.B - $r.T
$bmp = New-Object System.Drawing.Bitmap($w, $ht)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc()
# PW_RENDERFULLCONTENT (0x2): sem isso o WebView2 sai preto, porque o conteudo
# e composto fora da superficie classica da janela.
[void][W]::PrintWindow($h, $dc, 0x2)
$g.ReleaseHdc($dc); $g.Dispose()
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
Write-Output "$Out ${w}x${ht}"

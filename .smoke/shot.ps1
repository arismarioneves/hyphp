Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;using System.Runtime.InteropServices;
public class W {
 [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
 [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
 [StructLayout(LayoutKind.Sequential)] public struct R { public int L,T,Rt,B; }
}
'@
$p = Get-Process hyphp-dbg | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
[W+R]$r = New-Object W+R
[void][W]::GetWindowRect($p.MainWindowHandle, [ref]$r)
$b = New-Object System.Drawing.Bitmap(($r.Rt-$r.L), ($r.B-$r.T))
$g = [System.Drawing.Graphics]::FromImage($b); $dc = $g.GetHdc()
[void][W]::PrintWindow($p.MainWindowHandle, $dc, 0x2)
$g.ReleaseHdc($dc); $g.Dispose()
$b.Save($args[0], [System.Drawing.Imaging.ImageFormat]::Png); $b.Dispose()

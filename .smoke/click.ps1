param([int]$X,[int]$Y)
Add-Type @'
using System;using System.Runtime.InteropServices;
public class M {
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] public static extern void mouse_event(uint f,uint x,uint y,uint d,IntPtr e);
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h,int c);
 [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
 [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, IntPtr pid);
 [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a,uint b,bool at);
 [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
 [StructLayout(LayoutKind.Sequential)] public struct R { public int L,T,Rt,B; }
}
'@
$p = Get-Process hyphp-dbg | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
$h = $p.MainWindowHandle
$fg = [M]::GetForegroundWindow()
$t = [M]::GetWindowThreadProcessId($fg,[IntPtr]::Zero)
[void][M]::AttachThreadInput([M]::GetCurrentThreadId(),$t,$true)
[void][M]::ShowWindow($h,9); [void][M]::SetForegroundWindow($h)
[void][M]::AttachThreadInput([M]::GetCurrentThreadId(),$t,$false)
Start-Sleep -Milliseconds 600
[M+R]$r = New-Object M+R
[void][M]::GetWindowRect($h,[ref]$r)
$ax=$r.L+$X; $ay=$r.T+$Y
[void][M]::SetCursorPos(($ax-12),($ay-8)); Start-Sleep -Milliseconds 120
[void][M]::SetCursorPos($ax,$ay)
[M]::mouse_event(0x0001,0,0,0,[IntPtr]::Zero); Start-Sleep -Milliseconds 250
[M]::mouse_event(0x0002,0,0,0,[IntPtr]::Zero); Start-Sleep -Milliseconds 90
[M]::mouse_event(0x0004,0,0,0,[IntPtr]::Zero); Start-Sleep -Milliseconds 700

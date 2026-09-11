; ==============================================================================
; Nord Launcher - Modern NSIS Installer for Windows 10/11 (64-bit)
; ==============================================================================

!include "MUI2.nsh"
!include "x64.nsh"
!include "WinVer.nsh"

; General Configuration
Name "Nord Launcher"
OutFile "NordLauncher-Setup.exe"
Unicode True
RequestExecutionLevel user ; Non-elevated per-user installation in LocalAppData

; Default installation directory
InstallDir "$LOCALAPPDATA\Programs\NordLauncher"
InstallDirRegKey HKCU "Software\NordLauncher" "InstallDir"

; Interface Settings
!define MUI_ABORTWARNING
!define MUI_ICON "appicon.ico"
!define MUI_UNICON "appicon.ico"

; Pages
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; Uninstaller Pages
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; Languages
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Russian"

; Initialization & OS Verification Gate
Function .onInit
    ; 1. Require 64-bit Windows
    ${IfNot} ${RunningX64}
        MessageBox MB_OK|MB_ICONSTOP "Nord Launcher requires a 64-bit (x64) operating system."
        Abort
    ${EndIf}

    ; 2. Require Windows 10 (Build 1809+) or Windows 11. Strictly reject Win 7 / 8 / 8.1.
    ${IfNot} ${AtLeastWin10}
        MessageBox MB_OK|MB_ICONSTOP "Nord Launcher requires Windows 10 (version 1809 or higher) or Windows 11.$\r$\n$\r$\nLegacy Windows versions (7, 8, 8.1) are not supported."
        Abort
    ${EndIf}
FunctionEnd

; Installation Section
Section "Nord Launcher Core" SecCore
    SectionIn RO

    SetOutPath "$INSTDIR"
    File /r "..\..\dist\windows\*"

    ; Write registry configuration
    WriteRegStr HKCU "Software\NordLauncher" "InstallDir" "$INSTDIR"

    ; Windows Add/Remove Programs integration
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher" "DisplayName" "Nord Launcher"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher" "DisplayVersion" "0.1.0"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher" "Publisher" "Nord Launcher Team"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher" "DisplayIcon" "$INSTDIR\NordLauncher.exe,0"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher" "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher" "NoModify" 1
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher" "NoRepair" 1

    ; Create Shortcuts
    CreateDirectory "$SMPROGRAMS\Nord Launcher"
    CreateShortcut "$SMPROGRAMS\Nord Launcher\Nord Launcher.lnk" "$INSTDIR\NordLauncher.exe" "" "$INSTDIR\NordLauncher.exe" 0
    CreateShortcut "$SMPROGRAMS\Nord Launcher\Uninstall Nord Launcher.lnk" "$INSTDIR\uninstall.exe" "" "$INSTDIR\uninstall.exe" 0
    CreateShortcut "$DESKTOP\Nord Launcher.lnk" "$INSTDIR\NordLauncher.exe" "" "$INSTDIR\NordLauncher.exe" 0

    ; Write Uninstaller
    WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

; Uninstallation Section
Section "Uninstall"
    ; Remove Shortcuts
    Delete "$DESKTOP\Nord Launcher.lnk"
    Delete "$SMPROGRAMS\Nord Launcher\Nord Launcher.lnk"
    Delete "$SMPROGRAMS\Nord Launcher\Uninstall Nord Launcher.lnk"
    RMDir "$SMPROGRAMS\Nord Launcher"

    ; Remove Application Binaries
    Delete "$INSTDIR\NordLauncher.exe"
    Delete "$INSTDIR\uninstall.exe"
    RMDir /r "$INSTDIR"

    ; Remove Registry Keys
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher"
    DeleteRegKey HKCU "Software\NordLauncher"
SectionEnd

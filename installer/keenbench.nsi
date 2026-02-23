!include "MUI2.nsh"
!define ROOT_DIR "${__FILEDIR__}\.."
!define APP_NAME "KeenBench"
!define APP_EXE "keenbench.exe"

!ifndef APP_VERSION
!define APP_VERSION "0.0.0"
!endif

!ifndef APP_ICON
!define APP_ICON "${ROOT_DIR}\app\windows\runner\resources\app_icon.ico"
!endif

!ifndef APP_BUILD_DIR
!define APP_BUILD_DIR "${ROOT_DIR}\app\build\windows\x64\runner\Release"
!endif

OutFile "${APP_NAME}-Setup.exe"
InstallDir "$LOCALAPPDATA\${APP_NAME}"
InstallDirRegKey HKCU "Software\${APP_NAME}" "InstallDir"
RequestExecutionLevel user

Name "${APP_NAME} Installer"
Caption "${APP_NAME} Installer"

!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "${APP_NAME} Installer"
!define MUI_WELCOMEPAGE_TEXT "This wizard will install ${APP_NAME} on your computer.$\r$\n$\r$\nClick Next to continue."
!define MUI_FINISHPAGE_TEXT "Setup has finished installing ${APP_NAME} on your computer."

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "English"

VIProductVersion "${APP_VERSION}.0"
VIAddVersionKey /LANG=1033 "ProductName" "${APP_NAME}"
VIAddVersionKey /LANG=1033 "FileDescription" "${APP_NAME} Installer"
VIAddVersionKey /LANG=1033 "CompanyName" "${APP_NAME}"
VIAddVersionKey /LANG=1033 "FileVersion" "${APP_VERSION}"
VIAddVersionKey /LANG=1033 "ProductVersion" "${APP_VERSION}"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Copyright (c) 2026 Federico Keegan"

Icon "${APP_ICON}"
UninstallIcon "${APP_ICON}"

!define STARTMENU_DIR "$SMPROGRAMS\${APP_NAME}"
!define ARP_KEY "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${APP_NAME}"

Section "Install"
  SetOutPath "$INSTDIR"
  File /r "${APP_BUILD_DIR}\*"
  WriteRegStr HKCU "Software\${APP_NAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${ARP_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "${ARP_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "${ARP_KEY}" "Publisher" "${APP_NAME}"
  WriteRegStr HKCU "${ARP_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${ARP_KEY}" "UninstallString" "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "${ARP_KEY}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
  WriteRegDWORD HKCU "${ARP_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${ARP_KEY}" "NoRepair" 1
  CreateDirectory "${STARTMENU_DIR}"
  CreateShortCut "${STARTMENU_DIR}\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}"
  CreateShortCut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Uninstall"
  Delete "$DESKTOP\${APP_NAME}.lnk"
  Delete "${STARTMENU_DIR}\${APP_NAME}.lnk"
  RMDir "${STARTMENU_DIR}"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir /r "$INSTDIR"
  DeleteRegKey HKCU "Software\${APP_NAME}"
  DeleteRegKey HKCU "${ARP_KEY}"
SectionEnd

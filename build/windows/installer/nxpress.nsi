; The installer of NXPress for Windows. tools/installer makes it: it gives VERSION, NUMBERS,
; PROGRAM (the nxpress.exe), LICENSE, LANGS (the texts, from texts.json), IMAGES, ICON and
; OUTFILE.
;
; It installs for the user, in %LOCALAPPDATA%\Programs\NXPress, without the administrator. A
; folder only the administrator writes in (Program Files) is written by this same installer run
; as the administrator with /ELEVATED, which only copies the files; the shortcuts and what NXPress
; adds to Windows are made by the user's own installer, for the user. An NXPress installed
; already is updated in its folder, without asking again.

Unicode true
ManifestDPIAware true
ManifestSupportedOS all
RequestExecutionLevel user
SetCompressor /SOLID lzma

!include MUI2.nsh
!include LogicLib.nsh
!include FileFunc.nsh
!include nsDialogs.nsh
!include WinMessages.nsh

!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\NXPress"
!define SETTINGSKEY "Software\NXPress"
; the colors of NXPress: its dark band, the blue of what you choose, and the muted text
!define BAND "101216"
!define BLUE "3DB7EA"
!define BLUE_TEXT "0A6FB0"

Name "NXPress"
Caption "NXPress ${VERSION}"
UninstallCaption "NXPress ${VERSION}"
BrandingText "NXPress ${VERSION}"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\NXPress"
ShowInstDetails show
ShowUninstDetails show

VIProductVersion "${NUMBERS}"
VIFileVersion "${NUMBERS}"
VIAddVersionKey /LANG=0 "ProductName" "NXPress"
VIAddVersionKey /LANG=0 "FileDescription" "NXPress installer"
VIAddVersionKey /LANG=0 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "LegalCopyright" "AGPL-3.0"

Var OldVersion   ; the version installed, when there is one
Var Updating     ; 1: an NXPress is installed, and is updated in its folder
Var Elevated     ; 1: this is the installer run as the administrator, which only copies
Var NeedsAdmin   ; 1: the folder chosen is one only the administrator writes in
Var Result       ; the file the installer run as the administrator writes "ok" in
Var Dpi          ; 100, 125, 150, 175 or 200: the size of the images
Var Line         ; the blue line under the band of the header
Var LangCombo
Var DirNote
Var OptStart
Var OptDesktop
Var OptFiles
Var OptMenu
Var OptSendTo
Var UnKeep
Var UnKeepState

!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP "${IMAGES}\header-100.bmp"
!define MUI_HEADERIMAGE_BITMAP_STRETCH NoStretchNoCropNoAlign
!define MUI_HEADERIMAGE_UNBITMAP "${IMAGES}\header-100.bmp"
!define MUI_HEADERIMAGE_UNBITMAP_STRETCH NoStretchNoCropNoAlign
!define MUI_WELCOMEFINISHPAGE_BITMAP "${IMAGES}\side-100.bmp"
!define MUI_CUSTOMFUNCTION_GUIINIT GuiInit
!define MUI_CUSTOMFUNCTION_UNGUIINIT un.GuiInit

; 1 · welcome, with the language; or the update of the NXPress installed
!define MUI_PAGE_CUSTOMFUNCTION_SHOW WelcomeShow
!define MUI_WELCOMEPAGE_TITLE " "
!define MUI_WELCOMEPAGE_TEXT " "
!insertmacro MUI_PAGE_WELCOME
; 2 · the notice and the license
!define MUI_PAGE_CUSTOMFUNCTION_PRE SkipWhenUpdating
!define MUI_PAGE_CUSTOMFUNCTION_SHOW LicenseShow
!define MUI_LICENSEPAGE_TEXT_TOP "$(Notice)"
!define MUI_LICENSEPAGE_TEXT_BOTTOM "$(LicenseBottom)"
!insertmacro MUI_PAGE_LICENSE "${LICENSE}"
; 3 · the folder
!define MUI_PAGE_CUSTOMFUNCTION_PRE SkipWhenUpdating
!define MUI_PAGE_CUSTOMFUNCTION_SHOW DirShow
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE DirLeave
!define MUI_DIRECTORYPAGE_TEXT_TOP "$(DirTop)"
!insertmacro MUI_PAGE_DIRECTORY
; 4 · the options
Page custom OptionsPage OptionsLeave
; 5 · installing
!define MUI_PAGE_CUSTOMFUNCTION_SHOW InstFilesShow
!insertmacro MUI_PAGE_INSTFILES
; 6 · done, and open NXPress
!define MUI_PAGE_CUSTOMFUNCTION_SHOW FinishShow
!define MUI_FINISHPAGE_TITLE "$(FinishTitle)"
!define MUI_FINISHPAGE_TEXT "$(FinishText)"
!define MUI_FINISHPAGE_RUN "$INSTDIR\nxpress.exe"
!define MUI_FINISHPAGE_RUN_TEXT "$(FinishRun)"
!insertmacro MUI_PAGE_FINISH

; the uninstaller: what it takes away, and whether the settings stay
UninstPage custom un.KeepPage un.KeepLeave
!define MUI_PAGE_CUSTOMFUNCTION_SHOW un.InstFilesShow
!insertmacro MUI_UNPAGE_INSTFILES

!include "${LANGS}"

;--------------------------------
; the parts both use

; the size of the images for the scale of the screen
!macro PickDpi
  System::Call "user32::GetDpiForWindow(p $HWNDPARENT) i.r0"
  ${If} $0 < 1
    StrCpy $0 96
  ${EndIf}
  IntOp $0 $0 * 100
  IntOp $0 $0 / 96
  ${If} $0 >= 188
    StrCpy $Dpi 200
  ${ElseIf} $0 >= 163
    StrCpy $Dpi 175
  ${ElseIf} $0 >= 138
    StrCpy $Dpi 150
  ${ElseIf} $0 >= 113
    StrCpy $Dpi 125
  ${Else}
    StrCpy $Dpi 100
  ${EndIf}
!macroend

; the images of every size, written to the folder of the plug-ins
!macro ExtractImages
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File "/oname=side-100.bmp" "${IMAGES}\side-100.bmp"
  File "/oname=side-125.bmp" "${IMAGES}\side-125.bmp"
  File "/oname=side-150.bmp" "${IMAGES}\side-150.bmp"
  File "/oname=side-175.bmp" "${IMAGES}\side-175.bmp"
  File "/oname=side-200.bmp" "${IMAGES}\side-200.bmp"
  File "/oname=header-100.bmp" "${IMAGES}\header-100.bmp"
  File "/oname=header-125.bmp" "${IMAGES}\header-125.bmp"
  File "/oname=header-150.bmp" "${IMAGES}\header-150.bmp"
  File "/oname=header-175.bmp" "${IMAGES}\header-175.bmp"
  File "/oname=header-200.bmp" "${IMAGES}\header-200.bmp"
!macroend

; the header as a dark band: the logo at its left, the titles in white beside it, and a blue line
; under it
!macro StyleHeader
  !insertmacro PickDpi
  SetCtlColors $mui.Header.Background "" "${BAND}"
  SetCtlColors $mui.Header.Text "FFFFFF" "${BAND}"
  SetCtlColors $mui.Header.SubText "B9C3CF" "${BAND}"
  ; the place of the band in the window
  System::Call "*(i,i,i,i) p.r1"
  System::Call "user32::GetWindowRect(p $mui.Header.Background, p r1)"
  System::Call "user32::MapWindowPoints(p 0, p $HWNDPARENT, p r1, i 2)"
  System::Call "*$1(i.r2, i.r3, i.r4, i.r5)"
  System::Free $1
  IntOp $6 $5 - $3   ; its height
  IntOp $7 $4 - $2   ; its width
  ; the logo, as big as its image, at the left
  IntOp $8 64 * $Dpi
  IntOp $8 $8 / 100
  GetDlgItem $0 $HWNDPARENT 1046
  System::Call "user32::SetWindowPos(p r0, p 0, i r2, i r3, i r8, i r6, i 0x14)"
  SetBrandingImage /IMGID=1046 "$PLUGINSDIR\header-$Dpi.bmp"
  ; the titles beside it
  IntOp $9 $2 + $8
  IntOp $R0 $7 - $8
  IntOp $R0 $R0 - 8
  System::Call "*(i,i,i,i) p.r1"
  System::Call "user32::GetWindowRect(p $mui.Header.Text, p r1)"
  System::Call "user32::MapWindowPoints(p 0, p $HWNDPARENT, p r1, i 2)"
  System::Call "*$1(i, i.R1, i, i.R2)"
  IntOp $R2 $R2 - $R1
  System::Call "user32::SetWindowPos(p $mui.Header.Text, p 0, i r9, i R1, i R0, i R2, i 0x14)"
  System::Call "user32::GetWindowRect(p $mui.Header.SubText, p r1)"
  System::Call "user32::MapWindowPoints(p 0, p $HWNDPARENT, p r1, i 2)"
  System::Call "*$1(i, i.R1, i, i.R2)"
  System::Free $1
  IntOp $R2 $R2 - $R1
  IntOp $R3 $9 + 8
  IntOp $R0 $R0 - 8
  System::Call "user32::SetWindowPos(p $mui.Header.SubText, p 0, i R3, i R1, i R0, i R2, i 0x14)"
  ; the blue line, 3 pixels at 100 %, in place of the line of Windows
  IntOp $R4 3 * $Dpi
  IntOp $R4 $R4 / 100
  System::Call 'user32::CreateWindowExW(i 0, w "STATIC", w "", i 0x40000000, i r2, i r5, i r7, i R4, p $HWNDPARENT, p 0, p 0, p 0) p.s'
  Pop $Line
  SetCtlColors $Line "" "${BLUE}"
  ShowWindow $mui.Line.Standard ${SW_HIDE}
!macroend

; the image at the side of the first and the last page, for the scale of the screen
!macro SideImage IMAGE TITLE
  ${NSD_FreeImage} $mui.WelcomePage.Image.Bitmap
  ${NSD_SetImage} ${IMAGE} "$PLUGINSDIR\side-$Dpi.bmp" $mui.WelcomePage.Image.Bitmap
  SetCtlColors ${TITLE} "${BLUE_TEXT}" "FFFFFF"
  ShowWindow $Line ${SW_HIDE}
!macroend

; a label added to a page that MUI made, in its font
!macro AddLabel PAGE X Y W H TEXT
  System::Call "*(i ${X}, i ${Y}, i ${W}, i ${H}) p.r1"
  System::Call "user32::MapDialogRect(p ${PAGE}, p r1)"
  System::Call "*$1(i.r2, i.r3, i.r4, i.r5)"
  System::Free $1
  System::Call 'user32::CreateWindowExW(i 0, w "STATIC", w "${TEXT}", i 0x50000000, i r2, i r3, i r4, i r5, p ${PAGE}, p 0, p 0, p 0) p.s'
  Pop $0
  SendMessage ${PAGE} ${WM_GETFONT} 0 0 $1
  SendMessage $0 ${WM_SETFONT} $1 1
  SetCtlColors $0 "" "transparent"
!macroend

; whether a program is open: Windows keeps a running program open, and refuses to open it for
; someone alone
!macro IsRunning FILE OUT
  StrCpy ${OUT} 0
  ${If} ${FileExists} "${FILE}"
    System::Call "kernel32::CreateFileW(w '${FILE}', i 0x80000000, i 0, p 0, i 3, i 0x80, p 0) p.r0 ?e"
    Pop $1
    ${If} $0 = -1
      ${If} $1 = 32
        StrCpy ${OUT} 1
      ${EndIf}
    ${Else}
      System::Call "kernel32::CloseHandle(p r0)"
    ${EndIf}
  ${EndIf}
!macroend

; whether the user can write in a folder, made if it is not there
!macro CanWrite FOLDER OUT
  SetDetailsPrint none
  StrCpy ${OUT} 0
  ClearErrors
  CreateDirectory "${FOLDER}"
  ${IfNot} ${Errors}
    ClearErrors
    FileOpen $0 "${FOLDER}\.nxpress-installer-probe" w
    ${IfNot} ${Errors}
      FileClose $0
      Delete "${FOLDER}\.nxpress-installer-probe"
      StrCpy ${OUT} 1
    ${EndIf}
  ${EndIf}
  SetDetailsPrint both
!macroend

;--------------------------------
; the installer

Function .onInit
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/LANG=" $R1
  ${IfNot} ${Errors}
    StrCpy $LANGUAGE $R1
  ${EndIf}
  ClearErrors
  ${GetOptions} $R0 "/ELEVATED" $R1
  ${IfNot} ${Errors}
    ; run as the administrator: only the files, in the folder given with /D
    StrCpy $Elevated 1
    ${GetOptions} $R0 "/RESULT=" $Result
    SetSilent silent
    Return
  ${EndIf}
  ; an NXPress installed: its folder and its version
  ReadRegStr $0 HKCU "${UNINSTKEY}" "InstallLocation"
  ${If} $0 != ""
  ${AndIf} ${FileExists} "$0\nxpress.exe"
    StrCpy $Updating 1
    StrCpy $INSTDIR $0
    ReadRegStr $OldVersion HKCU "${UNINSTKEY}" "DisplayVersion"
    !insertmacro CanWrite "$INSTDIR" $1
    ${If} $1 = 0
      StrCpy $NeedsAdmin 1
    ${EndIf}
  ${EndIf}
  !insertmacro ExtractImages
FunctionEnd

Function GuiInit
  !insertmacro StyleHeader
FunctionEnd

Function SkipWhenUpdating
  ${If} $Updating = 1
    Abort
  ${EndIf}
FunctionEnd

Function WelcomeShow
  !insertmacro SideImage $mui.WelcomePage.Image $mui.WelcomePage.Title
  ${If} $Updating = 1
    SendMessage $mui.WelcomePage.Title ${WM_SETTEXT} 0 "STR:$(UpdateTitle)"
    SendMessage $mui.WelcomePage.Text ${WM_SETTEXT} 0 "STR:$(UpdateText)"
    SendMessage $mui.Button.Next ${WM_SETTEXT} 0 "STR:$(UpdateButton)"
    ${If} $NeedsAdmin = 1
      SendMessage $mui.Button.Next ${BCM_SETSHIELD} 0 1
    ${EndIf}
    StrCpy $R5 150
  ${Else}
    SendMessage $mui.WelcomePage.Title ${WM_SETTEXT} 0 "STR:$(WelcomeTitle)"
    SendMessage $mui.WelcomePage.Text ${WM_SETTEXT} 0 "STR:$(WelcomeText)"
    ${NSD_CreateLabel} 120u 172u 195u 12u "$(WelcomeNext)"
    Pop $0
    SetCtlColors $0 "" "FFFFFF"
    StrCpy $R5 110
  ${EndIf}
  ; the text ends where the language starts
  IntOp $R6 $R5 - 49
  System::Call "*(i 0, i 0, i 0, i R6) p.r1"
  System::Call "user32::MapDialogRect(p $mui.WelcomePage, p r1)"
  System::Call "*$1(i, i, i, i.r3)"
  System::Call "user32::GetWindowRect(p $mui.WelcomePage.Text, p r1)"
  System::Call "*$1(i.r4, i, i.r5, i)"
  System::Free $1
  IntOp $2 $5 - $4
  System::Call "user32::SetWindowPos(p $mui.WelcomePage.Text, p 0, i 0, i 0, i r2, i r3, i 0x16)"
  ${NSD_CreateLabel} 120u $R5u 195u 10u "$(Language)"
  Pop $0
  SetCtlColors $0 "" "FFFFFF"
  IntOp $R5 $R5 + 12
  ${NSD_CreateDropList} 120u $R5u 130u 120u ""
  Pop $LangCombo
  !insertmacro FillLanguages $LangCombo
  Call LangName
  ${NSD_CB_SelectString} $LangCombo $R9
  ${NSD_OnChange} $LangCombo LanguageChanged
FunctionEnd

; another language opens the installer again in it: NSIS keeps the language it opened with
Function LanguageChanged
  ${NSD_GetText} $LangCombo $R8
  Call LangByName
  ${If} $R9 != $LANGUAGE
    Exec '"$EXEPATH" /LANG=$R9'
    SendMessage $HWNDPARENT ${WM_CLOSE} 0 0
  ${EndIf}
FunctionEnd

Function LicenseShow
  ShowWindow $Line ${SW_SHOW}
  ; the notice takes the top, the license the rest
  System::Call "*(i 0, i 0, i 300, i 40) p.r1"
  System::Call "user32::MapDialogRect(p $mui.LicensePage, p r1)"
  System::Call "*$1(i, i, i.r2, i.r3)"
  System::Free $1
  System::Call "user32::SetWindowPos(p $mui.LicensePage.TopText, p 0, i 0, i 0, i r2, i r3, i 0x14)"
  System::Call "*(i,i,i,i) p.r1"
  System::Call "user32::GetWindowRect(p $mui.LicensePage.LicenseText, p r1)"
  System::Call "user32::MapWindowPoints(p 0, p $mui.LicensePage, p r1, i 2)"
  System::Call "*$1(i.r4, i.r5, i.r6, i.r7)"
  System::Free $1
  IntOp $8 $3 + 4
  IntOp $9 $7 - $8
  IntOp $6 $6 - $4
  System::Call "user32::SetWindowPos(p $mui.LicensePage.LicenseText, p 0, i r4, i r8, i r6, i r9, i 0x14)"
FunctionEnd

Function DirShow
  ShowWindow $Line ${SW_SHOW}
  !insertmacro AddLabel $mui.DirectoryPage 0 26 300 28 " "
  StrCpy $DirNote $0
  Call ShowDirNote
FunctionEnd

; the note under the folder: no administrator, or the question of Windows, with its shield on Next
Function ShowDirNote
  ${If} $DirNote == ""
    Return
  ${EndIf}
  StrCpy $R1 0
  ${ForEach} $R2 0 2 + 1
    ${If} $R2 = 0
      StrCpy $R3 $PROGRAMFILES64
    ${ElseIf} $R2 = 1
      StrCpy $R3 $PROGRAMFILES32
    ${Else}
      StrCpy $R3 $WINDIR
    ${EndIf}
    StrLen $R4 $R3
    StrCpy $R5 $INSTDIR $R4
    ${If} $R5 == $R3
      StrCpy $R1 1
    ${EndIf}
  ${Next}
  ${If} $R1 = 1
    SendMessage $DirNote ${WM_SETTEXT} 0 "STR:$(DirAdmin)"
  ${Else}
    SendMessage $DirNote ${WM_SETTEXT} 0 "STR:$(DirUser)"
  ${EndIf}
  SendMessage $mui.Button.Next ${BCM_SETSHIELD} 0 $R1
FunctionEnd

Function .onVerifyInstDir
  Call ShowDirNote
FunctionEnd

Function DirLeave
  StrCpy $DirNote ""
  SendMessage $mui.Button.Next ${BCM_SETSHIELD} 0 0
  !insertmacro CanWrite "$INSTDIR" $1
  ${If} $1 = 0
    StrCpy $NeedsAdmin 1
  ${Else}
    StrCpy $NeedsAdmin 0
  ${EndIf}
FunctionEnd

Function OptionsPage
  ${If} $Updating = 1
    Abort
  ${EndIf}
  ShowWindow $Line ${SW_SHOW}
  !insertmacro MUI_HEADER_TEXT "$(OptionsTitle)" "$(OptionsSub)"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateCheckbox} 0u 2u 300u 10u "$(OptStart)"
  Pop $OptStart
  ${NSD_Check} $OptStart
  ${NSD_CreateCheckbox} 0u 17u 300u 10u "$(OptDesktop)"
  Pop $OptDesktop
  ${NSD_Check} $OptDesktop
  ${NSD_CreateCheckbox} 0u 32u 300u 10u "$(OptFiles)"
  Pop $OptFiles
  ${NSD_Check} $OptFiles
  ${NSD_CreateLabel} 12u 43u 288u 10u "$(OptFilesHint)"
  Pop $0
  SetCtlColors $0 "666666" "transparent"
  ${NSD_CreateCheckbox} 0u 58u 300u 10u "$(OptMenu)"
  Pop $OptMenu
  ${NSD_CreateLabel} 12u 69u 288u 10u "$(OptMenuHint)"
  Pop $0
  SetCtlColors $0 "666666" "transparent"
  ${NSD_CreateCheckbox} 0u 84u 300u 10u "$(OptSendTo)"
  Pop $OptSendTo
  ${NSD_CreateLabel} 12u 95u 288u 10u "$(OptSendToHint)"
  Pop $0
  SetCtlColors $0 "666666" "transparent"
  ${If} $NeedsAdmin = 1
    SendMessage $mui.Button.Next ${BCM_SETSHIELD} 0 1
  ${EndIf}
  nsDialogs::Show
FunctionEnd

Function OptionsLeave
  ${NSD_GetState} $OptStart $OptStart
  ${NSD_GetState} $OptDesktop $OptDesktop
  ${NSD_GetState} $OptFiles $OptFiles
  ${NSD_GetState} $OptMenu $OptMenu
  ${NSD_GetState} $OptSendTo $OptSendTo
FunctionEnd

; the bar in the blue of NXPress: Windows paints its own green unless its theme is taken away
Function InstFilesShow
  ShowWindow $Line ${SW_SHOW}
  SendMessage $mui.Button.Next ${BCM_SETSHIELD} 0 0
  System::Call "uxtheme::SetWindowTheme(p $mui.InstFilesPage.ProgressBar, w ' ', w ' ')"
  SendMessage $mui.InstFilesPage.ProgressBar ${PBM_SETBARCOLOR} 0 0xEAB73D
  SendMessage $mui.InstFilesPage.ProgressBar ${PBM_SETBKCOLOR} 0 0xE6E6E6
FunctionEnd

Function FinishShow
  !insertmacro SideImage $mui.FinishPage.Image $mui.FinishPage.Title
FunctionEnd

; the files of NXPress in its folder
Function CopyFiles
  SetOutPath "$INSTDIR"
  File "/oname=nxpress.exe" "${PROGRAM}"
  File "/oname=LICENSE.txt" "${LICENSE}"
  ; what an update of NXPress left beside it
  Delete "$INSTDIR\nxpress.exe.old"
  Delete "$INSTDIR\nxpress.exe.*.old"
  WriteUninstaller "$INSTDIR\uninstall.exe"
FunctionEnd

Section "NXPress"
  ; the program and the rest, in KB, for Space required (files written in a function are not counted)
  AddSize ${SIZEKB}
  ${If} $Elevated = 1
    Call CopyFiles
    ${If} $Result != ""
      FileOpen $0 "$Result" w
      FileWrite $0 "ok"
      FileClose $0
    ${EndIf}
    Return
  ${EndIf}

  ; whether the folder needs the administrator, told again here for a silent install (/S /D=),
  ; which shows no page
  !insertmacro CanWrite "$INSTDIR" $1
  ${If} $1 = 0
    StrCpy $NeedsAdmin 1
  ${Else}
    StrCpy $NeedsAdmin 0
  ${EndIf}
  ; NXPress closed first: a program running cannot be replaced
  ${Do}
    !insertmacro IsRunning "$INSTDIR\nxpress.exe" $R0
    ${If} $R0 = 0
      ${Break}
    ${EndIf}
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(CloseApp)" /SD IDCANCEL IDRETRY +2
    Quit
  ${Loop}
  ${If} $NeedsAdmin = 1
    ; the files by this installer run as the administrator, which writes "ok" when it is done
    StrCpy $Result "$PLUGINSDIR\elevated-result.txt"
    DetailPrint "$INSTDIR"
    ${Do}
      SetDetailsPrint none
      Delete "$Result"
      ClearErrors
      ExecShellWait "runas" "$EXEPATH" '/ELEVATED /LANG=$LANGUAGE /RESULT="$Result" /D=$INSTDIR'
      SetDetailsPrint both
      ${IfNot} ${Errors}
        ${Break}
      ${EndIf}
      MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(AdminDeclined)" IDRETRY +2
      Quit
    ${Loop}
    StrCpy $0 ""
    ClearErrors
    FileOpen $1 "$Result" r
    ${IfNot} ${Errors}
      FileRead $1 $0
      FileClose $1
    ${EndIf}
    ${If} $0 != "ok"
      MessageBox MB_OK|MB_ICONSTOP "$(AdminFailed)"
      Quit
    ${EndIf}
  ${Else}
    Call CopyFiles
  ${EndIf}

  ; NXPress in Installed apps, for this user
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "NXPress"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\nxpress.exe,0"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "DeLFuS77"
  WriteRegStr HKCU "${UNINSTKEY}" "URLInfoAbout" "https://github.com/DeLFuS77/NXPress"
  WriteRegStr HKCU "${UNINSTKEY}" "HelpLink" "https://github.com/DeLFuS77/NXPress/wiki"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  WriteRegDWORD HKCU "${UNINSTKEY}" "EstimatedSize" $0
  ; the language, for the window of NXPress until one is chosen in it, and for the uninstaller
  Call LangCode
  WriteRegStr HKCU "${SETTINGSKEY}" "Language" $R9
  WriteRegStr HKCU "${SETTINGSKEY}" "InstallerLanguage" $LANGUAGE

  ${If} $Updating = 1
    Return
  ${EndIf}
  ${If} $OptStart = ${BST_CHECKED}
    CreateShortcut "$SMPROGRAMS\NXPress.lnk" "$INSTDIR\nxpress.exe"
  ${EndIf}
  ${If} $OptDesktop = ${BST_CHECKED}
    CreateShortcut "$DESKTOP\NXPress.lnk" "$INSTDIR\nxpress.exe"
  ${EndIf}
  ; the rest by NXPress, as its Settings do
  StrCpy $0 ""
  ${If} $OptFiles = ${BST_CHECKED}
    StrCpy $0 "$0 files"
  ${EndIf}
  ${If} $OptMenu = ${BST_CHECKED}
    StrCpy $0 "$0 menu"
  ${EndIf}
  ${If} $OptSendTo = ${BST_CHECKED}
    StrCpy $0 "$0 sendto"
  ${EndIf}
  ${If} $0 != ""
    nsExec::Exec '"$INSTDIR\nxpress.exe" --installer add$0 --language $R9'
    Pop $0
  ${EndIf}
SectionEnd

;--------------------------------
; the uninstaller

Function un.onInit
  ReadRegStr $0 HKCU "${SETTINGSKEY}" "InstallerLanguage"
  ${If} $0 != ""
    StrCpy $LANGUAGE $0
  ${EndIf}
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/ELEVATED" $R1
  ${IfNot} ${Errors}
    StrCpy $Elevated 1
    SetSilent silent
    Return
  ${EndIf}
  StrCpy $UnKeepState ${BST_CHECKED}
  !insertmacro ExtractImages
FunctionEnd

Function un.GuiInit
  !insertmacro StyleHeader
FunctionEnd

Function un.KeepPage
  ShowWindow $Line ${SW_SHOW}
  !insertmacro MUI_HEADER_TEXT "$(UnTitle)" "$(UnSub)"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0u 0u 300u 20u "$(UnText)"
  Pop $0
  ${NSD_CreateText} 0u 23u 300u 12u "$INSTDIR"
  Pop $0
  SendMessage $0 ${EM_SETREADONLY} 1 0
  ${NSD_CreateCheckbox} 0u 46u 300u 10u "$(UnKeep)"
  Pop $UnKeep
  ${NSD_SetState} $UnKeep $UnKeepState
  ${NSD_CreateLabel} 12u 57u 288u 28u "$(UnKeepHint)"
  Pop $0
  SetCtlColors $0 "666666" "transparent"
  ${NSD_CreateLabel} 0u 94u 300u 12u "$(UnClose)"
  Pop $0
  nsDialogs::Show
FunctionEnd

Function un.KeepLeave
  ${NSD_GetState} $UnKeep $UnKeepState
FunctionEnd

Function un.InstFilesShow
  ShowWindow $Line ${SW_SHOW}
  System::Call "uxtheme::SetWindowTheme(p $mui.InstFilesPage.ProgressBar, w ' ', w ' ')"
  SendMessage $mui.InstFilesPage.ProgressBar ${PBM_SETBARCOLOR} 0 0xEAB73D
  SendMessage $mui.InstFilesPage.ProgressBar ${PBM_SETBKCOLOR} 0 0xE6E6E6
FunctionEnd

; the files the installer put, and what updates left: whatever else is in the folder (a
; prod.keys put beside NXPress) stays, and the folder with it
Function un.DeleteFiles
  Delete "$INSTDIR\nxpress.exe"
  Delete "$INSTDIR\nxpress.exe.old"
  Delete "$INSTDIR\nxpress.exe.*.old"
  Delete "$INSTDIR\.nxpress-update-*"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
FunctionEnd

Section "Uninstall"
  ${If} $Elevated = 1
    Call un.DeleteFiles
    Return
  ${EndIf}
  ${Do}
    !insertmacro IsRunning "$INSTDIR\nxpress.exe" $R0
    ${If} $R0 = 0
      ${Break}
    ${EndIf}
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(CloseApp)" /SD IDCANCEL IDRETRY +2
    Quit
  ${Loop}
  ; what NXPress added to Windows, by NXPress, for this user: only what is for this copy
  ${If} ${FileExists} "$INSTDIR\nxpress.exe"
    nsExec::Exec '"$INSTDIR\nxpress.exe" --installer remove'
    Pop $0
  ${EndIf}
  ; the files: by this uninstaller run as the administrator when the folder is in Program Files
  !insertmacro CanWrite "$INSTDIR" $1
  ${If} $1 = 0
    ClearErrors
    SetDetailsPrint none
    ExecShellWait "runas" "$EXEPATH" '/ELEVATED _?=$INSTDIR'
    SetDetailsPrint both
    ${If} ${Errors}
      MessageBox MB_OK|MB_ICONSTOP "$(AdminDeclined)"
      Quit
    ${EndIf}
  ${Else}
    Call un.DeleteFiles
  ${EndIf}
  Delete "$SMPROGRAMS\NXPress.lnk"
  Delete "$DESKTOP\NXPress.lnk"
  DeleteRegKey HKCU "${UNINSTKEY}"
  DeleteRegValue HKCU "${SETTINGSKEY}" "Language"
  DeleteRegValue HKCU "${SETTINGSKEY}" "InstallerLanguage"
  DeleteRegKey /ifempty HKCU "${SETTINGSKEY}"
  ${If} $UnKeepState <> ${BST_CHECKED}
    ; the settings of NXPress, and the ones its page keeps
    RMDir /r "$APPDATA\NXPress"
    RMDir /r "$APPDATA\nxpress.exe"
  ${EndIf}
SectionEnd

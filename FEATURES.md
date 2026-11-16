# NXPress 1.7.16 - WICKED EDITION
Built on the official NXPress 1.7.16. Everything in the official release is still there, except the cover art icon of 1.7.11 and its crop of 1.7.12, which this edition replaces with its own (see Combine).


## Combine

- **Add a Cover Art Icon:** each dump folder in the list has one button with a menu: pick a picture from your computer, search the Nintendo eShop, or search SteamGridDB.
- **Pictures from your computer:** JPG, PNG, BMP, WebP, AVIF, GIF or ICO are allowed as attachments.
- **Crop Feature:** Drag and zoom to place the square that becomes the 256 × 256 icon, or tick "Fit the whole picture on white". The cover takes the place of the icon of the game's control data and of titledb.
- **Search Images from Nintendo eShop**: Filter by Switch 1 or Switch 2. Switch 2 games are shown first.
- **Search Images on SteamGridDB**: finds the game by name and offers its Icons (first), Grids, Heroes and Logos. It asks once for a free API key from your SteamGridDB profile and stores it in your settings (careful with sharing this folder).
- **Fixed a bug with Templates for NUP creation**: The Official version does not follow the template on the top right for NUP creation. This has been resolved. When you hit Combine, the NUP file name will follow the Template.
- **Template default changed:** {contents} has been removed from the template. {name} [{id}][v{version}] is now the default. Note: The program will remember the template if you change it.
- **NUP dropped in the wrong tool?** NXPress now tells you and points you to Split, Rename, Verify or Details.

## Rename

- **Edit Title & Icon** on each NUP in the list: Change the title name of the game and swap its icon inside the NUP, in place without unpacking it. Pick the icon from a file, the Nintendo eShop or SteamGridDB.
- The blue button reads **Rename based on Template**.

## Split

- **This is probably one of the most important features.**
- **Split now works with NUP files.** Just a note that Decompress tool only decompresses the NUP in its entirety. I used the Split tool to solve this.
- **Choose what to extract for each NUP:** You can now choose to extract specific parts of a NUP - Base, Update, DLC and/or Icon.
- **Checkmark System:** A checkmark box will appear next to each part (Base, Update, etc.). If a part is unavailable, its part will be disabled by default. This also serves as a visual indicator as to what is contained in the NUP.
- The game's icon is saved as `icon.jpg` in the root folder extraction. This also applies to the Decompress section.
- Testing on Donkey Kong Bananza with Base, Update, DLC, Icon. I Split with all parts selected, this took 13 minutes 16 seconds VS I split with only dlc selected, this took 31 seconds.

## Details

- **When selecting Show details:**
- Removed Change image button.
- Removed text line under the Change image button.
- Fixed alignment and centered green bordered Game, Update, and DLC (This is the first column).
- Fixed text wrapping in the second column which displays ID, Internal Build Number, External Build Number + a minor change removing the parenthesis and adding a ·.
- Added a second text line to go under the numerical values in the third column, fourth, fifth column.
- Capitalized "files" in the second text line in the third column.
- Added "Uncompressed" as a description in the second text line in the fourth column.
- Removed "as stored" and added "Compressed" in the second text line in the fifth column.
- Added a "Full Details" button where the change image button used to be. This will show you the report. A Copy full details button was also added.
- Completely re-worked the generated details report to give you a professional report, easy to understand and with text alignment. Also, added detection of icon and its size within the report.
- Added a .txt export to the export button at the top which will export our report to save it locally.

## Emulators

- **A completely new "Emulators" tool**  has been added under the "Log" tool on the left sidebar.
- **Starts empty,** with Nintendo Switch 1, Nintendo Switch 2 and Recompilations ready to fill.
- **What is a Section, Category, Subcategory?** Sections aka Nintendo Switch 1, Nintendo Switch 2, Recompilations. Categories aka Yuzu-Based, Ryujinx-Based. Subcategories aka Emulator names with their content.
- **Make it yours:** Add, edit and delete Sections, Categories and Subcategories, with a note for each if you like.
- **Collapse and expand:** Sections and Categories.
- **Drag and drop** to reorder section titles, categories and emulators.
- **Share and Import:** Save your list as a file or copy it as a code to send to a friend. Import a file or paste a code, then add to your list or replace it.
- **Reset to default** brings back the empty list.
- **Pre-Populate**: Import "NXPress emulators by WICKED <Date>.json" in the GitHub attachments to pre-populate your Emulators tool.

## Settings

- **Sidebar:** switch off any group or tool you don't use, and it disappears from the side menu.
- **Keys:** A change in its description so it is easier to understand. The line says "Select Change to Insert your prod.keys", and with no keys it says **No prod.keys detected** in red.

## Hover Card within Split, Rename, Verify, Details

- **Now displays Base, Update, DLC.** Each show the parts ID, Internal Build ID, and External Build ID.
- **Copy Details button** to easily copy the full report of the NUP.
- **View Details button** to open up an external window of the report for easy viewing. Includes a copy button and an export button.
- **Holds** has been moved to the bottom along with a divider line.
- **Spacing and bold** for a better visual appeal and it becomes easier to read.

## File NUP Display within Split, Rename, Verify, Details

- Minor re-work to only display Name, GameID under that, File Name under that.

## Miscellaneous

- **Multi-Window:** The ability to open multiple NXPress processes.

## Visual Changes on left sidebar when you expand the Window:

- Added Folder -> NUP description on the Combine Category or NUP -> Folder on the Split Category.
- Text has been capitalized or the descriptions have been changed.

## Source Code

- **Build Scripts:** Windows, Linux, Mac build scripts have been added to the source code to easily build NXPress.exe straight from the source code.
- **Settings:** A Settings Folder has been added to the source code by default. It has the ReadMe inside from the official developer.

# The window of the NXPress disk image, for dmgbuild (tools/installer runs it): NXPress.app on the
# left, the link to Applications on the right, on a background that says to drag one onto the
# other. dmgbuild takes background@2x.png beside background.png for Retina screens. Nothing in the
# image runs: it holds the app, the link and the hidden files of the Finder for this window.
#
# defines: app (the NXPress.app to put in it), background (background.png)

app = defines["app"]

format = "UDZO"
filesystem = "HFS+"
files = [(app, "NXPress.app")]
symlinks = {"Applications": "/Applications"}
hide_extensions = ["NXPress.app"]
icon = app + "/Contents/Resources/icon.icns"

background = defines["background"]
window_rect = ((200, 140), (540, 360))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
show_icon_preview = False
show_item_info = False
arrange_by = None
label_pos = "bottom"
icon_size = 96
text_size = 13
# the centres of the icons, where background.png has room for them
icon_locations = {"NXPress.app": (140, 170), "Applications": (400, 170)}

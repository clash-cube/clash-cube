"""Finder layout for the drag-to-install disk image (dmgbuild settings)."""

import os

application = os.path.abspath("bin/ClashCube.app")
files = [application]
symlinks = {"Applications": "/Applications"}
badge_icon = os.path.join(application, "Contents", "Resources", "icons.icns")
format = "UDZO"
background = defines["background"]
window_rect = ((200, 200), (640, 400))
icon_locations = {"ClashCube.app": (190, 205), "Applications": (450, 205)}
icon_size = 96
text_size = 16
default_view = "icon-view"
show_icon_preview = False
show_toolbar = False
show_status_bar = False
show_pathbar = False
show_sidebar = False
include_icon_view_settings = True
include_list_view_settings = False

package com.slopchop.jetbrains

import com.intellij.openapi.options.BoundConfigurable
import com.intellij.openapi.project.ProjectManager
import com.intellij.openapi.ui.DialogPanel
import com.intellij.ui.dsl.builder.bindSelected
import com.intellij.ui.dsl.builder.panel
import com.redhat.devtools.lsp4ij.LanguageServerManager

/**
 * SlopChopConfigurable is the settings page under Tools. Applying a change restarts the
 * slop-chop server in every open project, since the setting is part of its command line.
 */
class SlopChopConfigurable : BoundConfigurable("slop-chop") {
    private val options = SlopChopSettings.getInstance().state

    override fun createPanel(): DialogPanel = panel {
        row {
            checkBox("Check the comments in source files")
                .bindSelected(options::codeComments)
                .comment("Off, slop-chop checks Markdown and plain text only.")
        }
    }

    override fun apply() {
        super.apply()
        for (project in ProjectManager.getInstance().openProjects) {
            val manager = LanguageServerManager.getInstance(project)
            manager.stop(SERVER_ID)
            manager.start(SERVER_ID)
        }
    }
}

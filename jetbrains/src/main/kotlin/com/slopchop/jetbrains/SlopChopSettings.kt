package com.slopchop.jetbrains

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.PersistentStateComponent
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage

/** SlopChopSettings holds the plugin's settings for the whole IDE. */
@Service(Service.Level.APP)
@State(name = "SlopChopSettings", storages = [Storage("slop-chop.xml")])
class SlopChopSettings : PersistentStateComponent<SlopChopSettings.Options> {
    /** Options is the persisted settings state. */
    class Options {
        /** codeComments is whether the comments in source files are checked. */
        var codeComments: Boolean = true
    }

    private var options = Options()

    override fun getState(): Options = options

    override fun loadState(state: Options) {
        options = state
    }

    companion object {
        /** getInstance returns the IDE-wide settings. */
        fun getInstance(): SlopChopSettings =
            ApplicationManager.getApplication().getService(SlopChopSettings::class.java)
    }
}

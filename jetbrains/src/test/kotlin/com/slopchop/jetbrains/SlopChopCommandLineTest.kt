package com.slopchop.jetbrains

import com.intellij.testFramework.fixtures.BasePlatformTestCase

/**
 * SlopChopCommandLineTest checks the server command line built from each setting: comment
 * checking on by default, off as a flag, and the project folder as the working directory.
 */
class SlopChopCommandLineTest : BasePlatformTestCase() {
    fun testCommentsOn() {
        val cmd = commandLine("/work/repo", true)
        assertEquals(listOf("slop-chop", "lsp"), cmd.getCommandLineList(null))
        assertEquals("/work/repo", cmd.workDirectory?.path)
    }

    fun testCommentsOff() {
        val cmd = commandLine("/work/repo", false)
        assertEquals(listOf("slop-chop", "lsp", "--code-comments=false"), cmd.getCommandLineList(null))
    }

    fun testNoProjectFolder() {
        assertNull(commandLine(null, true).workDirectory)
    }

    fun testSettingsDefaultOn() {
        assertTrue(SlopChopSettings.Options().codeComments)
    }
}

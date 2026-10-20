package com.slopchop.jetbrains

import com.intellij.testFramework.fixtures.BasePlatformTestCase
import com.redhat.devtools.lsp4ij.LanguageServersRegistry

/**
 * SlopChopMappingTest proves that LSP4IJ hands the files plugin.xml maps to the slop-chop server
 * in a running platform, whatever language plugins the IDE ships, and leaves an unmapped type
 * alone.
 */
class SlopChopMappingTest : BasePlatformTestCase() {
    fun testGo() = assertSupported("a.go", true)

    fun testPython() = assertSupported("a.py", true)

    fun testTsx() = assertSupported("a.tsx", true)

    fun testUppercaseR() = assertSupported("script.R", true)

    fun testDockerfile() = assertSupported("Dockerfile", true)

    fun testMakefile() = assertSupported("Makefile", true)

    fun testPlainText() = assertSupported("notes.txt", true)

    fun testXmlUnmapped() = assertSupported("a.xml", false)

    /** assertSupported opens an empty file by that name and checks whether LSP4IJ maps it. */
    private fun assertSupported(name: String, want: Boolean) {
        val file = myFixture.configureByText(name, "")
        val got = LanguageServersRegistry.getInstance().isFileSupported(file)
        assertEquals("$name mapped to a language server", want, got)
    }
}

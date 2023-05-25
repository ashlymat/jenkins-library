package com.sap.piper

import org.junit.Rule
import org.junit.Before
import org.junit.Test
import static org.junit.Assert.assertEquals
import static org.junit.Assert.assertThat
import static org.junit.Assert.assertTrue
import org.junit.rules.ExpectedException
import org.junit.rules.RuleChain

import static org.hamcrest.Matchers.is
import static org.hamcrest.Matchers.hasItem
import static org.hamcrest.Matchers.hasItems
import static org.hamcrest.Matchers.hasSize

import util.JenkinsLoggingRule
import util.JenkinsShellCallRule
import util.BasePiperTest
import util.Rules

class UtilsTest extends BasePiperTest {
    private ExpectedException thrown = ExpectedException.none()
    private JenkinsLoggingRule loggingRule = new JenkinsLoggingRule(this)
    private JenkinsShellCallRule shellRule = new JenkinsShellCallRule(this)

    @Rule
    public RuleChain rules = Rules
        .getCommonRules(this)
        .around(thrown)
        .around(shellRule)
        .around(loggingRule)

    private parameters

    @Before
    void setup() {
        parameters = [:]
    }

    @Test
    void testGenerateSHA1() {
        def result = utils.generateSha1('ContinuousDelivery')
        // asserts
        // generated with "echo -n 'ContinuousDelivery' | sha1sum | sed 's/  -//'"
        assertThat(result, is('0dad6c33b6246702132454f604dee80740f399ad'))
    }

    @Test
    void testStashWithDefaults() {
        Map stashProperties

        def examinee = new Utils()
        examinee.steps = [
            stash: { Map stashProps ->
                stashProperties = stashProps
            },
        ]
        examinee.echo = {}

        examinee.stash('foo')

        assert(stashProperties == [name: 'foo', includes: '**/*.*', excludes: ''])
    }

   @Test
    void testStashWithIncludesAndExcludes() {
        Map stashProperties

        def examinee = new Utils()
        examinee.steps = [
            stash: { Map stashProps ->
                stashProperties = stashProps
            },
        ]
        examinee.echo = {}

        examinee.stash('foo', '**/*.mtar', '**/target')

        assert(stashProperties == [name: 'foo', includes: '**/*.mtar', excludes: '**/target'])
    }

    @Test
    void testUnstashStageFilesUnstashesAllUnstashableStashes() {

        // We do not fail in case a stash cannot be unstashed
        // That might be barely OK for non-existing stashes, but there might also be
        // real issues, e.g. related to permission issues when overwriting existing files
        // maybe also from other stashes unstashed earlier.
        // The behaviour wrt unstashable stashes should be improved. In case of issues
        // with unstashing, we should throw an exception

        boolean directoryDeleted = false
        def unstashed = []
        def examinee = new Utils()
        examinee.steps = [
            unstash: { def stashName ->
                if(stashName == 'fail') {
                    throw new RuntimeException('something went wrong')
                }
                unstashed << stashName
            },
        ]
        examinee.echo = {}

        nullScript.commonPipelineEnvironment.configuration.stageStashes = [
            foo : [
                unstash: ['stash-1', 'stash-2', 'fail', 'duplicate']
            ]
        ]

        nullScript.metaClass.deleteDir = { directoryDeleted = true }

        def stashResult = examinee.unstashStageFiles(nullScript, 'foo', ['additional-stash', 'duplicate'])

        assertThat(directoryDeleted, is(true))

        assertThat(unstashed, hasSize(5)) // should be for since we should not unstash 'duplicate' twice
        assertThat(unstashed, hasItems('stash-1', 'stash-2', 'additional-stash', 'duplicate'))

        // This is inconsistent. Above we can see only four different stashes has been unstashed (duplicate twice),
        // but here we see that the stashResult contains six entries, also the 'fail' entry
        // for which we throw an exception (... and duplicate twice).
        // We should fix that and adjust the test accordingly with the fix.
        assertThat(stashResult, hasSize(6))
        assertThat(stashResult, hasItems('stash-1', 'stash-2', 'additional-stash', 'fail', 'duplicate'))

        // cleanup the deleteDir method
        nullScript.metaClass = null
    }

    @Test
    void testUnstashAllSkipNull() {
        def stashResult = utils.unstashAll(['a', null, 'b'])
        assert stashResult == ['a', 'b']
    }

    @Test
    void testUnstashSkipsFailedUnstashes() {

        def examinee = new Utils()
        examinee.steps = [
            unstash: { def stashName ->
                if(stashName == 'fail') {
                    throw new RuntimeException('something went wrong')
                }
            },
        ]
        examinee.echo = {}

        def stashResult = examinee.unstashAll(['a', 'fail', 'b'])
        assert stashResult == ['a', 'b']
    }


    @Test
    void testUnstashAllSuceedsAndUnstashesWhatIsExpected() {
        def unstashed = [] as Set
        def examinee = new Utils()
        examinee.steps = [
            unstash: { def stashName -> unstashed << stashName},
        ]
        examinee.echo = {}
        examinee.unstashAll(['a', 'b'])

        assert(unstashed == ['a', 'b'] as Set)
    }

    @Test
    void testUnstashFails() {
        def logMessages = []
        def examinee = new Utils()
        examinee.steps = [
            unstash: { def stashName -> throw new RuntimeException('something went wrong')},
        ]
        examinee.echo = {
            // coerce to java.lang.String, we might have GStrings.
            // comparism with java.lang.String might fail.
            message -> logMessages << message.toString()
        }
        def stashResult = examinee.unstash('a')

        // in case unstash fails (maybe the stash does not exist, or we cannot unstash due to
        // some colliding files in conjunction with file permissions) we emit a log message
        // and continue silently instead of failing. In that case we get an empty array back
        // instead an array containing the name of the unstashed stash.
        assertThat(logMessages, hasItem('Unstash failed: a (something went wrong)'))
        assert(stashResult == [])
    }

    @Test
    void testAppendNonExistingParameterToStringList() {
        Map parameters = [:]
        List result = Utils.appendParameterToStringList([], parameters, 'non-existing')
        assertTrue(result.isEmpty())
    }

    @Test
    void testAppendStringParameterToStringList() {
        Map parameters = ['param': 'string']
        List result = Utils.appendParameterToStringList([], parameters, 'param')
        assertEquals(1, result.size())
    }

    @Test
    void testAppendListParameterToStringList() {
        Map parameters = ['param': ['string2', 'string3']]
        List result = Utils.appendParameterToStringList(['string1'], parameters, 'param')
        assertEquals(['string1', 'string2', 'string3'], result)
    }

    @Test
    void testAppendEmptyListParameterToStringList() {
        Map parameters = ['param': []]
        List result = Utils.appendParameterToStringList(['string'], parameters, 'param')
        assertEquals(['string'], result)
    }
}

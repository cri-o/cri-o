#!/usr/bin/env python3

# encoding: utf-8

# N/B: Assumes script was called from cri-o repository on the test subject,
#      with a remote name of 'origin.  It's executing under the results.yml
#      playbook, which in turn was executed by venv-ansible-playbook.sh
#      i.e. everything in requirements.txt is already available
#
# Also Requires:
#    python 2.7+
#    git

import os
import sys
import argparse
import re
import contextlib
import uuid
from socket import gethostname
import subprocess
from tempfile import NamedTemporaryFile
# Ref: https://github.com/gastlygem/junitparser
import junitparser

# Parser function suffixes and regex patterns of supported input filenames
TEST_TYPE_FILE_RE = dict(integration=re.compile(r'testout\.txt'),
                         e2e=re.compile(r'junit_\d+.xml'))
INTEGRATION_TEST_COUNT_RE = re.compile(r'^(?P<start>\d+)\.\.(?P<end>\d+)')
INTEGRATION_SKIP_RE = re.compile(r'^(?P<stat>ok|not ok) (?P<tno>\d+) # skip'
                                 r' (?P<sreason>\(.+\)) (?P<desc>.+)')
INTEGRATION_RESULT_RE = re.compile(r'^(?P<stat>ok|not ok) (?P<tno>\d+) (?P<desc>.+)')


def d(msg):
    if msg:
        try:
            sys.stderr.write('{}\n'.format(msg))
            sys.stderr.flush()
        except IOError:
            pass


@contextlib.contextmanager
def if_match(line, regex):
    # __enter__
    match = regex.search(line)
    if match:
        yield match
    else:
        yield None
    # __exit__
    pass  # Do nothing


def if_case_add(suite, line_parser, *parser_args, **parser_dargs):
    case = line_parser(*parser_args, **parser_dargs)
    if case:
        suite.add_testcase(case)


def parse_integration_line(line, classname):
    name_fmt = "[CRI-O] [integration] #{} {}"
    with if_match(line, INTEGRATION_SKIP_RE) as match:
        if match:
            name = name_fmt.format(match.group('tno'), match.group('desc'))
            case = junitparser.TestCase(name)
            case.classname = classname
            case.result = junitparser.Skipped(message=match.group('sreason'))
            case.system_err = match.group('stat')
            return case
    with if_match(line, INTEGRATION_RESULT_RE) as match:
        if match:
            name = name_fmt.format(match.group('tno'), match.group('desc'))
            case = junitparser.TestCase(name)
            case.classname = classname
            case.system_err = match.group('stat')
            if match.group('stat') == 'not ok':
                # Can't think of anything better to put here
                case.result = junitparser.Failed('not ok')
            elif not match.group('stat') == 'ok':
                case.result = junitparser.Error(match.group('stat'))
            return case
    return None


# N/B: name suffix corresponds to key in TEST_TYPE_FILE_RE
def parse_integration(input_file_path, hostname):
    suite = junitparser.TestSuite('CRI-O Integration suite')
    suite.hostname = hostname
    suite_stdout = []
    classname = 'CRI-O integration suite'
    n_tests = -1  # No tests ran
    d("    Processing integration results for {}".format(suite.hostname))
    with open(input_file_path) as testout_txt:
        for line in testout_txt:
            line = line.strip()
            suite_stdout.append(line)  # Basically a copy of the file
            # n_tests must come first
            with if_match(line, INTEGRATION_TEST_COUNT_RE) as match:
                if match:
                    n_tests = int(match.group('end')) - int(match.group('start')) + 1
                    d("      Collecting results from {} tests".format(n_tests))
                    break
        if n_tests > 0:
            for line in testout_txt:
                line = line.strip()
                suite_stdout.append(line)
                if_case_add(suite, parse_integration_line,
                            line=line, classname=classname)
        else:
            d("      Uh oh, no results found, skipping.")
            return None
    # TODO: No date/time recorded in file
    #stat = os.stat(input_file_path)
    #test_start = stat.st_mtime
    #test_end = stat.st_atime
    #duration = test_end - test_start
    suite.time = 0
    suite.add_property('stdout', '\n'.join(suite_stdout))

    d("    Parsed {} integration test cases".format(len(suite)))
    return suite


def flatten_testsuites(testsuites):
    # The jUnit format allows nesting testsuites, squash into a list for simplicity
    if isinstance(testsuites, junitparser.TestSuite):
        testsuite = testsuites  # for clarity
        return [testsuite]
    result = []
    for testsuite in testsuites:
        if isinstance(testsuite, junitparser.TestSuite):
            result.append(testsuite)
        elif isinstance(testsuite, junitparser.JUnitXml):
            nested_suites = flatten_testsuites(testsuite)
            if nested_suites:
                result += nested_suites
    return result


def find_k8s_e2e_suite(testsuites):
    testsuites = flatten_testsuites(testsuites)
    for testsuite in testsuites:
        if testsuite.name and 'Kubernetes e2e' in testsuite.name:
            return testsuite
        # Name could be None or wrong, check classnames of all tests
        classnames = ['Kubernetes e2e' in x.classname.strip() for x in testsuite]
        if all(classnames):
            return testsuite
    return None


# N/B: name suffix corresponds to key in TEST_TYPE_FILE_RE
def parse_e2e(input_file_path, hostname):
    # Load junit_xx.xml file, update contents with more identifying info.
    try:
        testsuites = junitparser.JUnitXml.fromfile(input_file_path)
        suite = find_k8s_e2e_suite(testsuites)
    except junitparser.JUnitXmlError as xcept:
        d("    Error parsing {}, skipping it.: {}".format(input_file_path, xcept))
        return None
    if not suite:
        d("    Failed to find any e2e results in {}".format(input_file_path))
        return None
    if not suite.hostname:
        suite.hostname = hostname
    if not suite.name:
        suite.name = 'Kubernetes e2e suite'
    d("    Processing e2e results for {}".format(suite.hostname))
    for testcase in suite:
        if not testcase.classname:
            d("    Adding missing classname to case {}".format(testcase.name))
            testcase.classname = "Kubernetes e2e suite"
    d("    Parsed {} e2e test cases".format(len(suite)))
    if not suite.time:
        stat = os.stat(input_file_path)
        test_start = stat.st_ctime
        test_end = stat.st_mtime
        duration = test_end - test_start
        if duration:
            suite.time = duration
    return testsuites  # Retain original structure

def parse_test_output(ifps, results_name, hostname):
    time_total = 0
    testsuites = junitparser.JUnitXml(results_name)
    # Cheat, lookup parser function name suffix from global namespace
    _globals = globals()
    for input_file_path in ifps:
        if not os.path.isfile(input_file_path):
            d(" The file {} doesn't appear to exist, skipping it.".format(input_file_path))
            continue
        parser = None
        for tname, regex in TEST_TYPE_FILE_RE.items():
            if regex.search(input_file_path):
                parser = _globals.get('parse_{}'.format(tname))
                break
        else:
            d(" Could not find parser to handle input"
              " file {}, skipping.".format(input_file_path))
            continue

        d("  Parsing {} using {}".format(input_file_path, parser))
        for parsed_testsuite in flatten_testsuites(parser(input_file_path, hostname)):
            d("  Adding {} suite for {}".format(parsed_testsuite.name, parsed_testsuite.hostname))
            testsuites.add_testsuite(parsed_testsuite)
            if parsed_testsuite.time:
                time_total += parsed_testsuite.time
    testsuites.time = time_total
    return testsuites

def make_host_name():
    subject = '{}'.format(gethostname())
    # Origin-CI doesn't use very distinguishable hostnames :(
    if 'openshiftdevel' in subject or 'ip-' in subject:
        try:
            with open('/etc/machine-id') as machineid:
                subject = 'machine-id-{}'.format(machineid.read().strip())
        except IOError:  # Worst-case, but we gotta pick sumpfin
            subject = 'uuid-{}'.format(uuid.uuid4())
    return subject

def make_results_name(argv):
    script_dir = os.path.dirname(argv[0])
    spco = lambda cmd: subprocess.check_output(cmd.split(' '),
                                               stderr=subprocess.STDOUT,
                                               close_fds=True,
                                               cwd=script_dir,
                                               universal_newlines=True)
    pr_no = None
    head_id = None
    try:
        head_id = spco('git rev-parse HEAD')
        for line in spco('git ls-remote origin refs/pull/[0-9]*/head').strip().splitlines():
            cid, ref = line.strip().split(None, 1)
            if head_id in cid:
                pr_no = ref.strip().split('/')[2]
                break
    except subprocess.CalledProcessError:
        pass

    if pr_no:
        return "CRI-O Pull Request {}".format(pr_no)
    elif head_id:
        return "CRI-O Commit {}".format(head_id[:8])
    else:  # Worst-case, but we gotta pick sumpfin
        return "CRI-O Run ID {}".format(uuid.uuid4())


def main(argv):
    reload(sys)
    sys.setdefaultencoding('utf8')
    parser = argparse.ArgumentParser(epilog='Note: The parent directory of input files is'
                                            'assumed to be the test suite name')
    parser.add_argument('-f', '--fqdn',
                        help="Alternative hostname to add to results if none present",
                        default=make_host_name())
    parser.add_argument('-b', '--backup', action="store_true",
                        help="If output file name matches any input file, backup with"
                             " 'original_' prefix",
                        default=False)
    parser.add_argument('ifps', nargs='+',
                        help='Input file paths to test output from {}.'
                             ''.format(TEST_TYPE_FILE_RE.keys()))
    parser.add_argument('ofp', nargs=1,
                        default='-',
                        help='Output file path for jUnit XML, or "-" for stdout')
    options = parser.parse_args(argv[1:])
    ofp = options.ofp[0]  # nargs==1 still puts it into a list
    results_name = make_results_name(argv)

    d("Parsing {} to {}".format(options.ifps, ofp))
    d("Using results name: {} and hostname {}".format(results_name, options.fqdn))
    # Parse all results
    new_testsuites = parse_test_output(options.ifps, results_name, options.fqdn)

    if not len(new_testsuites):
        d("Uh oh, doesn't look like anything was processed.  Bailing out")
        return None

    d("Parsed {} suites".format(len(new_testsuites)))

    # etree can't handle files w/o filenames :(
    tmp = NamedTemporaryFile(suffix='.tmp', prefix=results_name, bufsize=1)
    new_testsuites.write(tmp.name)
    tmp.seek(0)
    del new_testsuites  # close up any open files
    if ofp == '-':
        sys.stdout.write('\n{}\n'.format(tmp.read()))
    else:
        for ifp in options.ifps:
            if not os.path.isfile(ofp):
                break
            if os.path.samefile(ifp, ofp):
                if not options.backup:
                    d("Warning {} will be will be combined with other input files."
                      "".format(ofp))
                    break
                dirname = os.path.dirname(ofp)
                basename = os.path.basename(ofp)
                origname = 'original_{}'.format(basename)
                os.rename(ofp, os.path.join(dirname, origname))
                break
        with open(ofp, 'w', 1) as output_file:
            output_file.truncate(0)
            output_file.flush()
            d("Writing {}".format(ofp))
            output_file.write(tmp.read())


if __name__ == '__main__':
    main(sys.argv)

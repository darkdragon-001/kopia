package cli

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/compression"
	"github.com/kopia/kopia/snapshot/policy"
)

var booleanEnumValues = []string{"true", "false", "inherit"}

var (
	policySetCommand = policyCommands.Command("set", "Set snapshot policy for a single directory, user@host or a global policy.")
	policySetTargets = policySetCommand.Arg("target", "Target of a policy ('global','user@host','@host') or a path").Strings()
	policySetGlobal  = policySetCommand.Flag("global", "Set global policy").Bool()

	// Frequency.
	policySetInterval   = policySetCommand.Flag("snapshot-interval", "Interval between snapshots").DurationList()
	policySetTimesOfDay = policySetCommand.Flag("snapshot-time", "Times of day when to take snapshot (HH:mm)").Strings()

	// Expiration policies.
	policySetKeepLatest  = policySetCommand.Flag("keep-latest", "Number of most recent backups to keep per source (or 'inherit')").PlaceHolder("N").String()
	policySetKeepHourly  = policySetCommand.Flag("keep-hourly", "Number of most-recent hourly backups to keep per source (or 'inherit')").PlaceHolder("N").String()
	policySetKeepDaily   = policySetCommand.Flag("keep-daily", "Number of most-recent daily backups to keep per source (or 'inherit')").PlaceHolder("N").String()
	policySetKeepWeekly  = policySetCommand.Flag("keep-weekly", "Number of most-recent weekly backups to keep per source (or 'inherit')").PlaceHolder("N").String()
	policySetKeepMonthly = policySetCommand.Flag("keep-monthly", "Number of most-recent monthly backups to keep per source (or 'inherit')").PlaceHolder("N").String()
	policySetKeepAnnual  = policySetCommand.Flag("keep-annual", "Number of most-recent annual backups to keep per source (or 'inherit')").PlaceHolder("N").String()

	// Files to ignore.
	policySetAddIgnore    = policySetCommand.Flag("add-ignore", "List of paths to add to the ignore list").PlaceHolder("PATTERN").Strings()
	policySetRemoveIgnore = policySetCommand.Flag("remove-ignore", "List of paths to remove from the ignore list").PlaceHolder("PATTERN").Strings()
	policySetClearIgnore  = policySetCommand.Flag("clear-ignore", "Clear list of paths in the ignore list").Bool()

	policyIgnoreCacheDirs = policySetCommand.Flag("ignore-cache-dirs", "Ignore cache directories ('true', 'false', 'inherit')").Enum(booleanEnumValues...)

	// Name of compression algorithm.
	policySetCompressionAlgorithm = policySetCommand.Flag("compression", "Compression algorithm").Enum(supportedCompressionAlgorithms()...)
	policySetCompressionMinSize   = policySetCommand.Flag("compression-min-size", "Min size of file to attempt compression for").String()
	policySetCompressionMaxSize   = policySetCommand.Flag("compression-max-size", "Max size of file to attempt compression for").String()

	// Files to only compress.
	policySetAddOnlyCompress    = policySetCommand.Flag("add-only-compress", "List of extensions to add to the only-compress list").PlaceHolder("PATTERN").Strings()
	policySetRemoveOnlyCompress = policySetCommand.Flag("remove-only-compress", "List of extensions to remove from the only-compress list").PlaceHolder("PATTERN").Strings()
	policySetClearOnlyCompress  = policySetCommand.Flag("clear-only-compress", "Clear list of extensions in the only-compress list").Bool()

	// Files to never compress.
	policySetAddNeverCompress    = policySetCommand.Flag("add-never-compress", "List of extensions to add to the never compress list").PlaceHolder("PATTERN").Strings()
	policySetRemoveNeverCompress = policySetCommand.Flag("remove-never-compress", "List of extensions to remove from the never compress list").PlaceHolder("PATTERN").Strings()
	policySetClearNeverCompress  = policySetCommand.Flag("clear-never-compress", "Clear list of extensions in the never compress list").Bool()

	// Dot-ignore files to look at.
	policySetAddDotIgnore    = policySetCommand.Flag("add-dot-ignore", "List of paths to add to the dot-ignore list").PlaceHolder("FILENAME").Strings()
	policySetRemoveDotIgnore = policySetCommand.Flag("remove-dot-ignore", "List of paths to remove from the dot-ignore list").PlaceHolder("FILENAME").Strings()
	policySetClearDotIgnore  = policySetCommand.Flag("clear-dot-ignore", "Clear list of paths in the dot-ignore list").Bool()
	policySetMaxFileSize     = policySetCommand.Flag("max-file-size", "Exclude files above given size").PlaceHolder("N").String()

	// Ignore other mounted fileystems.
	policyOneFileSystem = policySetCommand.Flag("one-file-system", "Stay in parent filesystem when finding files ('true', 'false', 'inherit')").Enum(booleanEnumValues...)

	// Error handling behavior.
	policyIgnoreFileErrors      = policySetCommand.Flag("ignore-file-errors", "Ignore errors reading files while traversing ('true', 'false', 'inherit')").Enum(booleanEnumValues...)
	policyIgnoreDirectoryErrors = policySetCommand.Flag("ignore-dir-errors", "Ignore errors reading directories while traversing ('true', 'false', 'inherit").Enum(booleanEnumValues...)

	// General policy.
	policySetInherit = policySetCommand.Flag(inheritPolicyString, "Enable or disable inheriting policies from the parent").BoolList()
)

const (
	inheritPolicyString = "inherit"
)

func init() {
	policySetCommand.Action(repositoryAction(setPolicy))
}

func setPolicy(ctx context.Context, rep repo.Repository) error {
	targets, err := policyTargets(ctx, rep, policySetGlobal, policySetTargets)
	if err != nil {
		return err
	}

	for _, target := range targets {
		p, err := policy.GetDefinedPolicy(ctx, rep, target)

		switch {
		case errors.Is(err, policy.ErrPolicyNotFound):
			p = &policy.Policy{}
		case err != nil:
			return errors.Wrap(err, "could not get defined policy")
		}

		log(ctx).Infof("Setting policy for %v\n", target)

		changeCount := 0
		if err := setPolicyFromFlags(ctx, p, &changeCount); err != nil {
			return err
		}

		if changeCount == 0 {
			return errors.New("no changes specified")
		}

		if err := policy.SetPolicy(ctx, rep, target, p); err != nil {
			return errors.Wrapf(err, "can't save policy for %v", target)
		}
	}

	return nil
}

func setPolicyFromFlags(ctx context.Context, p *policy.Policy, changeCount *int) error {
	if err := setRetentionPolicyFromFlags(ctx, &p.RetentionPolicy, changeCount); err != nil {
		return errors.Wrap(err, "retention policy")
	}

	if err := setFilesPolicyFromFlags(ctx, &p.FilesPolicy, changeCount); err != nil {
		return errors.Wrap(err, "files policy")
	}

	if err := setErrorHandlingPolicyFromFlags(ctx, &p.ErrorHandlingPolicy, changeCount); err != nil {
		return errors.Wrap(err, "error handling policy")
	}

	if err := setCompressionPolicyFromFlags(ctx, &p.CompressionPolicy, changeCount); err != nil {
		return errors.Wrap(err, "compression policy")
	}

	if err := setSchedulingPolicyFromFlags(ctx, &p.SchedulingPolicy, changeCount); err != nil {
		return errors.Wrap(err, "scheduling policy")
	}

	if err := applyPolicyNumber64(ctx, "maximum file size", &p.FilesPolicy.MaxFileSize, *policySetMaxFileSize, changeCount); err != nil {
		return errors.Wrap(err, "maximum file size")
	}

	// It's not really a list, just optional boolean, last one wins.
	for _, inherit := range *policySetInherit {
		*changeCount++

		p.NoParent = !inherit
	}

	return nil
}

func setFilesPolicyFromFlags(ctx context.Context, fp *policy.FilesPolicy, changeCount *int) error {
	if *policySetClearDotIgnore {
		*changeCount++

		log(ctx).Infof(" - removing all rules for dot-ignore files\n")

		fp.DotIgnoreFiles = nil
	} else {
		fp.DotIgnoreFiles = addRemoveDedupeAndSort(ctx, "dot-ignore files", fp.DotIgnoreFiles, *policySetAddDotIgnore, *policySetRemoveDotIgnore, changeCount)
	}

	if *policySetClearIgnore {
		*changeCount++

		fp.IgnoreRules = nil

		log(ctx).Infof(" - removing all ignore rules\n")
	} else {
		fp.IgnoreRules = addRemoveDedupeAndSort(ctx, "ignored files", fp.IgnoreRules, *policySetAddIgnore, *policySetRemoveIgnore, changeCount)
	}

	switch {
	case *policyIgnoreCacheDirs == "":
	case *policyIgnoreCacheDirs == inheritPolicyString:
		*changeCount++

		fp.IgnoreCacheDirs = nil

		log(ctx).Infof(" - inherit ignoring cache dirs from parent\n")

	default:
		val, err := strconv.ParseBool(*policyIgnoreCacheDirs)
		if err != nil {
			return err
		}

		*changeCount++

		fp.IgnoreCacheDirs = &val

		log(ctx).Infof(" - setting ignore cache dirs to %v\n", val)
	}

	switch {
	case *policyOneFileSystem == "":
	case *policyOneFileSystem == inheritPolicyString:
		*changeCount++

		fp.OneFileSystem = nil

		printStderr(" - inherit one file system from parent\n")

	default:
		val, err := strconv.ParseBool(*policyOneFileSystem)
		if err != nil {
			return err
		}

		*changeCount++

		fp.OneFileSystem = &val

		printStderr(" - setting one file system to %v\n", val)
	}

	return nil
}

func setErrorHandlingPolicyFromFlags(ctx context.Context, fp *policy.ErrorHandlingPolicy, changeCount *int) error {
	switch {
	case *policyIgnoreFileErrors == "":
	case *policyIgnoreFileErrors == inheritPolicyString:
		*changeCount++

		fp.IgnoreFileErrors = nil

		log(ctx).Infof(" - inherit file read error behavior from parent\n")
	default:
		val, err := strconv.ParseBool(*policyIgnoreFileErrors)
		if err != nil {
			return err
		}

		*changeCount++

		fp.IgnoreFileErrors = &val

		log(ctx).Infof(" - setting ignore file read errors to %v\n", val)
	}

	switch {
	case *policyIgnoreDirectoryErrors == "":
	case *policyIgnoreDirectoryErrors == inheritPolicyString:
		*changeCount++

		fp.IgnoreDirectoryErrors = nil

		log(ctx).Infof(" - inherit directory read error behavior from parent\n")
	default:
		val, err := strconv.ParseBool(*policyIgnoreDirectoryErrors)
		if err != nil {
			return err
		}

		*changeCount++

		fp.IgnoreDirectoryErrors = &val

		log(ctx).Infof(" - setting ignore directory read errors to %v\n", val)
	}

	return nil
}

func setRetentionPolicyFromFlags(ctx context.Context, rp *policy.RetentionPolicy, changeCount *int) error {
	cases := []struct {
		desc      string
		max       **int
		flagValue *string
	}{
		{"number of annual backups to keep", &rp.KeepAnnual, policySetKeepAnnual},
		{"number of monthly backups to keep", &rp.KeepMonthly, policySetKeepMonthly},
		{"number of weekly backups to keep", &rp.KeepWeekly, policySetKeepWeekly},
		{"number of daily backups to keep", &rp.KeepDaily, policySetKeepDaily},
		{"number of hourly backups to keep", &rp.KeepHourly, policySetKeepHourly},
		{"number of latest backups to keep", &rp.KeepLatest, policySetKeepLatest},
	}

	for _, c := range cases {
		if err := applyPolicyNumber(ctx, c.desc, c.max, *c.flagValue, changeCount); err != nil {
			return err
		}
	}

	return nil
}

func setSchedulingPolicyFromFlags(ctx context.Context, sp *policy.SchedulingPolicy, changeCount *int) error {
	// It's not really a list, just optional value.
	for _, interval := range *policySetInterval {
		*changeCount++

		sp.SetInterval(interval)
		log(ctx).Infof(" - setting snapshot interval to %v\n", sp.Interval())

		break
	}

	if len(*policySetTimesOfDay) > 0 {
		var timesOfDay []policy.TimeOfDay

		for _, tods := range *policySetTimesOfDay {
			for _, tod := range strings.Split(tods, ",") {
				if tod == inheritPolicyString {
					timesOfDay = nil
					break
				}

				var timeOfDay policy.TimeOfDay
				if err := timeOfDay.Parse(tod); err != nil {
					return errors.Wrap(err, "unable to parse time of day")
				}

				timesOfDay = append(timesOfDay, timeOfDay)
			}
		}
		*changeCount++

		sp.TimesOfDay = policy.SortAndDedupeTimesOfDay(timesOfDay)

		if timesOfDay == nil {
			log(ctx).Infof(" - resetting snapshot times of day to default\n")
		} else {
			log(ctx).Infof(" - setting snapshot times to %v\n", timesOfDay)
		}
	}

	return nil
}

func setCompressionPolicyFromFlags(ctx context.Context, p *policy.CompressionPolicy, changeCount *int) error {
	if err := applyPolicyNumber64(ctx, "minimum file size subject to compression", &p.MinSize, *policySetCompressionMinSize, changeCount); err != nil {
		return errors.Wrap(err, "minimum file size subject to compression")
	}

	if err := applyPolicyNumber64(ctx, "maximum file size subject to compression", &p.MaxSize, *policySetCompressionMaxSize, changeCount); err != nil {
		return errors.Wrap(err, "maximum file size subject to compression")
	}

	if v := *policySetCompressionAlgorithm; v != "" {
		*changeCount++

		if v == inheritPolicyString {
			log(ctx).Infof(" - resetting compression algorithm to default value inherited from parent\n")

			p.CompressorName = ""
		} else {
			log(ctx).Infof(" - setting compression algorithm to %v\n", v)

			p.CompressorName = compression.Name(v)
		}
	}

	if *policySetClearOnlyCompress {
		*changeCount++

		p.OnlyCompress = nil

		log(ctx).Infof(" - removing all only-compress extensions\n")
	} else {
		p.OnlyCompress = addRemoveDedupeAndSort(ctx, "only-compress extensions",
			p.OnlyCompress, *policySetAddOnlyCompress, *policySetRemoveOnlyCompress, changeCount)
	}

	if *policySetClearNeverCompress {
		*changeCount++

		p.NeverCompress = nil

		log(ctx).Infof(" - removing all never-compress extensions\n")
	} else {
		p.NeverCompress = addRemoveDedupeAndSort(ctx, "never-compress extensions",
			p.NeverCompress, *policySetAddNeverCompress, *policySetRemoveNeverCompress, changeCount)
	}

	return nil
}

func addRemoveDedupeAndSort(ctx context.Context, desc string, base, add, remove []string, changeCount *int) []string {
	entries := map[string]bool{}
	for _, b := range base {
		entries[b] = true
	}

	for _, b := range add {
		*changeCount++

		log(ctx).Infof(" - adding %v to %v\n", b, desc)

		entries[b] = true
	}

	for _, b := range remove {
		*changeCount++

		log(ctx).Infof(" - removing %v from %v\n", b, desc)
		delete(entries, b)
	}

	var s []string
	for k := range entries {
		s = append(s, k)
	}

	sort.Strings(s)

	return s
}

func applyPolicyNumber(ctx context.Context, desc string, val **int, str string, changeCount *int) error {
	if str == "" {
		// not changed
		return nil
	}

	if str == inheritPolicyString || str == "default" {
		*changeCount++

		log(ctx).Infof(" - resetting %v to a default value inherited from parent.\n", desc)

		*val = nil

		return nil
	}

	v, err := strconv.ParseInt(str, 10, 32)
	if err != nil {
		return errors.Wrapf(err, "can't parse the %v %q", desc, str)
	}

	i := int(v)
	*changeCount++

	log(ctx).Infof(" - setting %v to %v.\n", desc, i)
	*val = &i

	return nil
}

func applyPolicyNumber64(ctx context.Context, desc string, val *int64, str string, changeCount *int) error {
	if str == "" {
		// not changed
		return nil
	}

	if str == inheritPolicyString || str == "default" {
		*changeCount++

		log(ctx).Infof(" - resetting %v to a default value inherited from parent.\n", desc)

		*val = 0

		return nil
	}

	v, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return errors.Wrapf(err, "can't parse the %v %q", desc, str)
	}

	*changeCount++

	log(ctx).Infof(" - setting %v to %v.\n", desc, v)
	*val = v

	return nil
}

func supportedCompressionAlgorithms() []string {
	var res []string
	for name := range compression.ByName {
		res = append(res, string(name))
	}

	sort.Strings(res)

	return append([]string{inheritPolicyString, "none"}, res...)
}

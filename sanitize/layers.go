package sanitize

// Layers holds what a run's profile is built from on top of the built-in default, so the
// command line and the editor engines resolve one precedence rule instead of each keeping a
// copy of it.
type Layers struct {
	// Project is the profile file a run found or was given, or nil when there is none. It
	// extends the built-in default unless it says standalone.
	Project *Profile
	// Dialect overrides the profile's own dialect when set.
	Dialect string
	// Presets are merged over the profile, which still wins any conflict.
	Presets []string
	// Voice is the personal layer, or the zero Voice when there is none.
	Voice Voice
}

// Profile resolves the layers into one profile. The order is the built-in default, the
// project file, the dialect, the presets, and then the voice, whose prefer swaps win and
// whose keep list silences preset cuts. A project file still outranks a voice, so it is
// applied again on top of one.
func (l Layers) Profile() (Profile, error) {
	profile := DefaultProfile()
	var project Profile
	if l.Project != nil {
		// A profile extends the built-in default unless it says standalone. The two-line
		// team file, allow one word and ban another, means the default detection plus those
		// two decisions, not a profile of two rules.
		project = *l.Project
		if !project.Standalone {
			project = DefaultProfile().Overlay(project)
		}
		profile = project
	}
	if l.Dialect != "" {
		profile.Dialect = Dialect(l.Dialect)
	}
	if len(l.Presets) > 0 {
		merged, err := ApplyPresets(profile, l.Presets...)
		if err != nil {
			return Profile{}, err
		}
		profile = merged
	}
	if !l.Voice.Empty() {
		profile = profile.WithVoice(l.Voice)
		if l.Project != nil {
			profile = profile.Overlay(project)
		}
	}
	return profile, nil
}

// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

type SettingsSuite struct {
	internalStateSuite
	key        string
	collection string
}

var _ = gc.Suite(&SettingsSuite{})

func (s *SettingsSuite) SetUpTest(c *gc.C) {
	s.internalStateSuite.SetUpTest(c)
	s.key = "config"
	s.collection = settingsC
}

func (s *SettingsSuite) createSettings(key string, values map[string]interface{}) (*Settings, error) {
	return createSettings(s.state.db(), s.collection, key, values)
}

func (s *SettingsSuite) readSettings() (*Settings, error) {
	return readSettings(s.state.db(), s.collection, s.key)
}

func (s *SettingsSuite) TestCreateEmptySettings(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.Keys(), gc.DeepEquals, []string{})
}

func (s *SettingsSuite) TestCannotOverwrite(c *gc.C) {
	_, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.createSettings(s.key, nil)
	c.Assert(err, gc.ErrorMatches, "cannot overwrite existing settings")
}

func (s *SettingsSuite) TestCannotReadMissing(c *gc.C) {
	_, err := s.readSettings()
	c.Assert(err, gc.ErrorMatches, "settings not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SettingsSuite) TestCannotWriteMissing(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = removeSettings(s.state.db(), s.collection, s.key)
	c.Assert(err, jc.ErrorIsNil)

	node.Set("foo", "bar")
	_, err = node.Write()
	c.Assert(err, gc.ErrorMatches, "settings not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *SettingsSuite) TestUpdateWithWrite(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Update(options)
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})

	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)

	// Check MongoDB state.
	var mgoData struct {
		Settings settingsMap
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(map[string]interface{}(mgoData.Settings), gc.DeepEquals, options)
}

func (s *SettingsSuite) TestConflictOnSet(c *gc.C) {
	// Check version conflict errors.
	nodeOne, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)

	optionsOld := map[string]interface{}{"alpha": "beta", "one": 1}
	nodeOne.Update(optionsOld)
	nodeOne.Write()

	nodeTwo.Update(optionsOld)
	changes, err := nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})

	// First test node one.
	c.Assert(nodeOne.Map(), gc.DeepEquals, optionsOld)

	// Write on node one.
	optionsNew := map[string]interface{}{"alpha": "gamma", "one": "two"}
	nodeOne.Update(optionsNew)
	changes, err = nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemModified, "alpha", "beta", "gamma"},
		{ItemModified, "one", 1, "two"},
	})

	// Verify that node one reports as expected.
	c.Assert(nodeOne.Map(), gc.DeepEquals, optionsNew)

	// Verify that node two has still the old data.
	c.Assert(nodeTwo.Map(), gc.DeepEquals, optionsOld)

	// Now issue a Set/Write from node two. This will
	// merge the data deleting 'one' and updating
	// other values.
	optionsMerge := map[string]interface{}{"alpha": "cappa", "new": "next"}
	nodeTwo.Update(optionsMerge)
	nodeTwo.Delete("one")

	expected := map[string]interface{}{"alpha": "cappa", "new": "next"}
	changes, err = nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemModified, "alpha", "beta", "cappa"},
		{ItemAdded, "new", nil, "next"},
		{ItemDeleted, "one", 1, nil},
	})
	c.Assert(expected, gc.DeepEquals, nodeTwo.Map())

	// But node one still reflects the former data.
	c.Assert(nodeOne.Map(), gc.DeepEquals, optionsNew)
}

func (s *SettingsSuite) TestSetItem(c *gc.C) {
	// Check that Set works as expected.
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Set("alpha", "beta")
	node.Set("one", 1)
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})
	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)
	// Check MongoDB state.
	var mgoData struct {
		Settings settingsMap
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(map[string]interface{}(mgoData.Settings), gc.DeepEquals, options)
}

func (s *SettingsSuite) TestSetItemEscape(c *gc.C) {
	// Check that Set works as expected.
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	options := map[string]interface{}{"$bar": 1, "foo.alpha": "beta"}
	node.Set("foo.alpha", "beta")
	node.Set("$bar", 1)
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "$bar", nil, 1},
		{ItemAdded, "foo.alpha", nil, "beta"},
	})
	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)

	// Check MongoDB state.
	mgoOptions := map[string]interface{}{"\uff04bar": 1, "foo\uff0ealpha": "beta"}
	var mgoData struct {
		Settings map[string]interface{}
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mgoData.Settings, gc.DeepEquals, mgoOptions)

	// Now get another state by reading from the database instance and
	// check read state has replaced '.' and '$' after fetching from
	// MongoDB.
	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodeTwo.disk, gc.DeepEquals, options)
	c.Assert(nodeTwo.core, gc.DeepEquals, options)
}

func (s *SettingsSuite) TestRawSettingsMapEncodeDecode(c *gc.C) {
	smap := &settingsMap{
		"$dollar":    1,
		"dotted.key": 2,
	}
	asBSON, err := bson.Marshal(smap)
	c.Assert(err, jc.ErrorIsNil)
	var asMap map[string]interface{}
	// unmarshalling into a map doesn't do the custom decoding so we get the raw escaped keys
	err = bson.Unmarshal(asBSON, &asMap)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(asMap, gc.DeepEquals, map[string]interface{}{
		"\uff04dollar":    1,
		"dotted\uff0ekey": 2,
	})
	// unmarshalling into a settingsMap will give us the right decoded keys
	var asSettingsMap settingsMap
	err = bson.Unmarshal(asBSON, &asSettingsMap)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(map[string]interface{}(asSettingsMap), gc.DeepEquals, map[string]interface{}{
		"$dollar":    1,
		"dotted.key": 2,
	})
}

func (s *SettingsSuite) TestReplaceSettingsEscape(c *gc.C) {
	// Check that replaceSettings works as expected.
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	node.Set("foo.alpha", "beta")
	node.Set("$bar", 1)
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)

	options := map[string]interface{}{"$baz": 1, "foo.bar": "beta"}
	rop, settingsChanged, err := replaceSettingsOp(s.state.db(), s.collection, s.key, options)
	c.Assert(err, jc.ErrorIsNil)
	ops := []txn.Op{rop}
	err = node.db.RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	changed, err := settingsChanged()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changed, jc.IsTrue)

	// Check MongoDB state.
	mgoOptions := map[string]interface{}{"\uff04baz": 1, "foo\uff0ebar": "beta"}
	var mgoData struct {
		Settings map[string]interface{}
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mgoData.Settings, gc.DeepEquals, mgoOptions)
}

func (s *SettingsSuite) TestcreateSettingsEscape(c *gc.C) {
	// Check that createSettings works as expected.
	options := map[string]interface{}{"$baz": 1, "foo.bar": "beta"}
	node, err := s.createSettings(s.key, options)
	c.Assert(err, jc.ErrorIsNil)

	// Check local state.
	c.Assert(node.Map(), gc.DeepEquals, options)

	// Check MongoDB state.
	mgoOptions := map[string]interface{}{"\uff04baz": 1, "foo\uff0ebar": "beta"}
	var mgoData struct {
		Settings map[string]interface{}
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()

	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mgoData.Settings, gc.DeepEquals, mgoOptions)
}

func (s *SettingsSuite) TestMultipleReads(c *gc.C) {
	// Check that reads without writes always resets the data.
	nodeOne, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	value, ok := nodeOne.Get("alpha")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "bar")
	value, ok = nodeOne.Get("baz")
	c.Assert(ok, jc.IsFalse)

	// A read resets the data to the empty state.
	err = nodeOne.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodeOne.Map(), gc.DeepEquals, map[string]interface{}{})
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	changes, err := nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "foo", nil, "bar"},
	})

	// A write retains the newly set values.
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "bar")

	// Now get another state instance and change underlying state.
	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	nodeTwo.Update(map[string]interface{}{"foo": "different"})
	changes, err = nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemModified, "foo", "bar", "different"},
	})

	// This should pull in the new state into node one.
	err = nodeOne.Read()
	c.Assert(err, jc.ErrorIsNil)
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "different")
}

func (s *SettingsSuite) TestDeleteEmptiesState(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	node.Set("a", "foo")
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})
	node.Delete("a")
	changes, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemDeleted, "a", "foo", nil},
	})
	c.Assert(node.Map(), gc.DeepEquals, map[string]interface{}{})
}

func (s *SettingsSuite) TestReadResync(c *gc.C) {
	// Check that read pulls the data into the node.
	nodeOne, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})
	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	nodeTwo.Delete("a")
	changes, err = nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemDeleted, "a", "foo", nil},
	})
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "bar"},
	})
	// Read of node one should pick up the new value.
	err = nodeOne.Read()
	c.Assert(err, jc.ErrorIsNil)
	value, ok := nodeOne.Get("a")
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "bar")
}

func (s *SettingsSuite) TestMultipleWrites(c *gc.C) {
	// Check that multiple writes only do the right changes.
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	changes, err := node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "foo", nil, "bar"},
		{ItemAdded, "this", nil, "that"},
	})
	node.Delete("this")
	node.Set("another", "value")
	changes, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "another", nil, "value"},
		{ItemDeleted, "this", "that", nil},
	})

	expected := map[string]interface{}{"foo": "bar", "another": "value"}
	c.Assert(expected, gc.DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{})

	err = node.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(expected, gc.DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{})
}

func (s *SettingsSuite) TestMultipleWritesAreStable(c *gc.C) {
	node, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)

	var mgoData struct {
		Settings map[string]interface{}
	}
	settings, closer := s.state.db().GetCollection(settingsC)
	defer closer()
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	version := mgoData.Settings["version"]
	for i := 0; i < 100; i++ {
		node.Set("value", i)
		node.Set("foo", "bar")
		node.Delete("value")
		node.Set("this", "that")
		_, err := node.Write()
		c.Assert(err, jc.ErrorIsNil)
	}
	mgoData.Settings = make(map[string]interface{})
	err = settings.FindId(s.key).One(&mgoData)
	c.Assert(err, jc.ErrorIsNil)
	newVersion := mgoData.Settings["version"]
	c.Assert(version, gc.Equals, newVersion)
}

func (s *SettingsSuite) TestWriteTwice(c *gc.C) {
	// Check the correct writing into a node by two config nodes.
	nodeOne, err := s.createSettings(s.key, nil)
	c.Assert(err, jc.ErrorIsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})

	nodeTwo, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{
		{ItemModified, "a", "foo", "bar"},
	})

	// Shouldn't write again. Changes were already
	// flushed and acted upon by other parties.
	changes, err = nodeOne.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.DeepEquals, []ItemChange{})

	err = nodeOne.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodeOne.key, gc.Equals, nodeTwo.key)
	c.Assert(nodeOne.disk, gc.DeepEquals, nodeTwo.disk)
	c.Assert(nodeOne.core, gc.DeepEquals, nodeTwo.core)
}

func (s *SettingsSuite) TestList(c *gc.C) {
	_, err := s.createSettings("key#1", map[string]interface{}{"foo1": "bar1"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.createSettings("key#2", map[string]interface{}{"foo2": "bar2"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.createSettings("another#1", map[string]interface{}{"foo2": "bar2"})
	c.Assert(err, jc.ErrorIsNil)

	nodes, err := listSettings(s.state, s.collection, "key#")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodes, jc.DeepEquals, map[string]map[string]interface{}{
		"key#1": {"foo1": "bar1"},
		"key#2": {"foo2": "bar2"},
	})
}

func (s *SettingsSuite) TestUpdatingInterfaceSliceValue(c *gc.C) {
	// When storing config values that are coerced from schemas as
	// List(Something), the value will always be a []interface{}. Make
	// sure we can safely update settings with those values.
	s1, err := s.createSettings(s.key, map[string]interface{}{
		"foo1": []interface{}{"bar1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s1.Write()
	c.Assert(err, jc.ErrorIsNil)

	s2, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	s2.Set("foo1", []interface{}{"bar1", "bar2"})
	_, err = s2.Write()
	c.Assert(err, jc.ErrorIsNil)

	s3, err := s.readSettings()
	c.Assert(err, jc.ErrorIsNil)
	value, found := s3.Get("foo1")
	c.Assert(found, gc.Equals, true)
	c.Assert(value, gc.DeepEquals, []interface{}{"bar1", "bar2"})
}
